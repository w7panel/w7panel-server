package k8s

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	yamlreader "k8s.io/apimachinery/pkg/util/yaml"
	typedcore "k8s.io/client-go/kubernetes/typed/core/v1"
	"sigs.k8s.io/yaml"
)

// preservePVCStorageRenderer preserves live PVC requests during Helm upgrades.
// Values remain unchanged; the corrected manifest is stored by Helm.
type preservePVCStorageRenderer struct {
	ctx       context.Context
	client    typedcore.CoreV1Interface
	namespace string
}

func (r *preservePVCStorageRenderer) Run(input *bytes.Buffer) (*bytes.Buffer, error) {
	original := append([]byte(nil), input.Bytes()...)
	reader := yamlreader.NewYAMLReader(bufio.NewReader(bytes.NewReader(original)))
	output := &bytes.Buffer{}
	for {
		document, err := reader.Read()
		if err == io.EOF {
			return output, nil
		}
		if err != nil {
			slog.Warn("PVC post-render read failed, preserving original manifest", "namespace", r.namespace, "error", err)
			return bytes.NewBuffer(original), nil
		}
		rendered, err := r.renderDocument(document)
		if err != nil {
			slog.Warn("PVC post-render failed, preserving original resource", "namespace", r.namespace, "error", err)
			rendered = document
		}
		output.WriteString("---\n")
		output.Write(rendered)
		if len(rendered) > 0 && rendered[len(rendered)-1] != '\n' {
			output.WriteByte('\n')
		}
	}
}

func (r *preservePVCStorageRenderer) renderDocument(document []byte) ([]byte, error) {
	object := &unstructured.Unstructured{}
	if err := yaml.Unmarshal(document, &object.Object); err != nil {
		return nil, fmt.Errorf("decode Helm manifest: %w", err)
	}
	if object.GetKind() != "PersistentVolumeClaim" {
		return document, nil
	}
	namespace := object.GetNamespace()
	if namespace == "" {
		namespace = r.namespace
	}
	claim, err := r.client.PersistentVolumeClaims(namespace).Get(r.ctx, object.GetName(), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return document, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read PVC %s/%s: %w", namespace, object.GetName(), err)
	}
	current, ok := claim.Spec.Resources.Requests[corev1.ResourceStorage]
	if !ok || current.Sign() <= 0 {
		return nil, fmt.Errorf("PVC %s/%s has no valid storage request", namespace, object.GetName())
	}
	requested, _, err := unstructured.NestedString(object.Object, "spec", "resources", "requests", "storage")
	if err != nil {
		return nil, fmt.Errorf("read rendered PVC %s/%s storage: %w", namespace, object.GetName(), err)
	}
	quantity, parseErr := resource.ParseQuantity(requested)
	if parseErr == nil && quantity.Cmp(current) == 0 {
		return document, nil
	}
	if err := unstructured.SetNestedField(object.Object, current.String(), "spec", "resources", "requests", "storage"); err != nil {
		return nil, fmt.Errorf("set PVC %s/%s storage: %w", namespace, object.GetName(), err)
	}
	rendered, err := yaml.Marshal(object.Object)
	if err != nil {
		return nil, fmt.Errorf("encode PVC %s/%s: %w", namespace, object.GetName(), err)
	}
	return rendered, nil
}
