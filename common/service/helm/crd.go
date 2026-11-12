package helm

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
)

var crdResource = schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}

const CRDCleanupAnnotation = "w7.cc/uninstall-crds"
const CRDCleanupSignatureAnnotation = "w7.cc/uninstall-crds-signature"
const CRDCleanupAfterGroupsAnnotation = "w7.cc/uninstall-crds-after-groups"
const CRDCleanupForceSharedAnnotation = "w7.cc/uninstall-crds-force-shared"

func CRDCleanupSignature(namespace, uid, raw string) (string, error) {
	key := strings.TrimSpace(os.Getenv("PANEL_AUTH_SIGNING_KEY"))
	if key == "" {
		key = strings.TrimSpace(facade.Config.GetString("panel_auth.signing_key"))
	}
	if key == "" {
		key = strings.TrimSpace(facade.Config.GetString("app.aeskey"))
	}
	if key == "" {
		return "", errors.New("CRD cleanup signing key is not configured")
	}
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(namespace + "\n" + uid + "\n" + raw))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func VerifyCRDCleanupSignature(namespace, uid, raw, signature string) bool {
	expected, err := CRDCleanupSignature(namespace, uid, raw)
	if err != nil {
		return false
	}
	provided, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	want, _ := hex.DecodeString(expected)
	return hmac.Equal(want, provided)
}

// ChartCRDs reads only Helm's standard crds/ directory, including subcharts.
func ChartCRDs(ch *chart.Chart) (map[string]*unstructured.Unstructured, error) {
	if ch == nil {
		return nil, errors.New("Helm chart is missing")
	}
	result := map[string]*unstructured.Unstructured{}
	for _, file := range ch.CRDObjects() {
		decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(file.File.Data), 4096)
		for {
			var object map[string]interface{}
			if err := decoder.Decode(&object); err != nil {
				if err == io.EOF {
					break
				}
				return nil, fmt.Errorf("parse %s: %w", file.Filename, err)
			}
			if len(object) == 0 {
				continue
			}
			crd := &unstructured.Unstructured{Object: object}
			if crd.GetAPIVersion() != "apiextensions.k8s.io/v1" || crd.GetKind() != "CustomResourceDefinition" || crd.GetName() == "" {
				return nil, fmt.Errorf("%s is not a v1 CustomResourceDefinition", file.Filename)
			}
			if _, exists := result[crd.GetName()]; exists {
				return nil, fmt.Errorf("duplicate CRD %s", crd.GetName())
			}
			result[crd.GetName()] = crd
		}
	}
	return result, nil
}

func ChartDigest(ch *chart.Chart) (string, error) {
	if ch == nil {
		return "", errors.New("Helm chart is missing")
	}
	var content func(*chart.Chart) interface{}
	content = func(current *chart.Chart) interface{} {
		dependencies := make([]interface{}, 0, len(current.Dependencies()))
		for _, dependency := range current.Dependencies() {
			dependencies = append(dependencies, content(dependency))
		}
		return []interface{}{current.Metadata, current.Lock, current.Templates, current.Values, current.Schema, current.Files, dependencies}
	}
	encoded, err := json.Marshal(content(ch))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// ProcessChartDependencies applies Helm's typed --set semantics to dependency conditions.
func ProcessChartDependencies(ch *chart.Chart, set, setJSON []string) error {
	vals, err := (&values.Options{Values: set, JSONValues: setJSON}).MergeValues(getter.All(cli.New()))
	if err != nil {
		return err
	}
	return chartutil.ProcessDependenciesWithMerge(ch, vals)
}

func ChangedCRDs(ctx context.Context, sdk *k8s.Sdk, crds map[string]*unstructured.Unstructured) ([]string, error) {
	return changedCRDs(ctx, sdk.DynamicClient(), crds)
}

func changedCRDs(ctx context.Context, client dynamic.Interface, crds map[string]*unstructured.Unstructured) ([]string, error) {
	names := make([]string, 0, len(crds))
	for name, desired := range crds {
		live, err := client.Resource(crdResource).Get(ctx, name, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return nil, err
		}
		if apierrors.IsNotFound(err) || !equalCRDSpec(live.Object["spec"], desired.Object["spec"]) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func equalCRDSpec(a, b interface{}) bool {
	decode := func(value interface{}) (apiextensionsv1.CustomResourceDefinitionSpec, error) {
		var spec apiextensionsv1.CustomResourceDefinitionSpec
		encoded, err := json.Marshal(value)
		if err != nil {
			return spec, err
		}
		err = json.Unmarshal(encoded, &spec)
		apiextensionsv1.SetDefaults_CustomResourceDefinitionSpec(&spec)
		return spec, err
	}
	left, err1 := decode(a)
	right, err2 := decode(b)
	return err1 == nil && err2 == nil && reflect.DeepEqual(left, right)
}

func ApplyCRDs(ctx context.Context, sdk *k8s.Sdk, crds map[string]*unstructured.Unstructured) error {
	return applyCRDs(ctx, sdk.DynamicClient(), crds)
}

func applyCRDs(ctx context.Context, dynamicClient dynamic.Interface, crds map[string]*unstructured.Unstructured) error {
	client := dynamicClient.Resource(crdResource)
	changed := []string{}
	for name, desired := range crds {
		live, err := client.Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			if _, err = client.Create(ctx, desired, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("create CRD %s: %w", name, err)
			}
			changed = append(changed, name)
			continue
		}
		if err != nil {
			return err
		}
		if equalCRDSpec(live.Object["spec"], desired.Object["spec"]) {
			continue
		}
		live.Object["spec"] = desired.Object["spec"]
		if _, err = client.Update(ctx, live, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update CRD %s: %w", name, err)
		}
		changed = append(changed, name)
	}
	for _, name := range changed {
		err := wait.PollUntilContextTimeout(ctx, time.Second, time.Minute, true, func(ctx context.Context) (bool, error) {
			crd, err := client.Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			conditions, _, _ := unstructured.NestedSlice(crd.Object, "status", "conditions")
			for _, raw := range conditions {
				condition, ok := raw.(map[string]interface{})
				if ok && condition["type"] == "Established" && condition["status"] == "True" {
					return true, nil
				}
			}
			return false, nil
		})
		if err != nil {
			return fmt.Errorf("wait for CRD %s: %w", name, err)
		}
	}
	return nil
}

func DeleteCRDs(ctx context.Context, sdk *k8s.Sdk, names []string) error {
	return deleteCRDs(ctx, sdk.DynamicClient(), names)
}

func deleteCRDs(ctx context.Context, client dynamic.Interface, names []string) error {
	for _, name := range names {
		err := client.Resource(crdResource).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete CRD %s: %w", name, err)
		}
		if apierrors.IsNotFound(err) {
			continue
		}
		if err := wait.PollUntilContextTimeout(ctx, time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
			_, err := client.Resource(crdResource).Get(ctx, name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}); err != nil {
			return fmt.Errorf("wait for CRD %s deletion: %w", name, err)
		}
	}
	return nil
}

type UninstallCRDResult struct {
	Names []string `json:"names"`
	Risks []string `json:"risks"`
}

// UninstallCRDs uses the installed chart; the remote chart may have changed.
func (h *Helm) UninstallCRDs(ctx context.Context, namespace, releaseName string) (UninstallCRDResult, error) {
	release, err := h.Info(releaseName, namespace)
	if err != nil {
		if errors.Is(err, driver.ErrReleaseNotFound) {
			return UninstallCRDResult{Names: []string{}, Risks: []string{}}, nil
		}
		return UninstallCRDResult{}, err
	}
	crds, err := ChartCRDs(release.Chart)
	if err != nil {
		return UninstallCRDResult{}, err
	}
	result := UninstallCRDResult{Names: []string{}, Risks: []string{}}
	for name := range crds {
		result.Names = append(result.Names, name)
	}
	sort.Strings(result.Names)
	if len(result.Names) == 0 {
		return result, nil
	}
	result.Risks = h.SharedCRDRisks(ctx, result.Names, releaseName, namespace)
	return result, nil
}

func (h *Helm) SharedCRDRisks(ctx context.Context, names []string, releaseName, namespace string) []string {
	risks := []string{}
	selected := make(map[string]bool, len(names))
	for _, name := range names {
		selected[name] = true
	}
	releases, err := h.listCRDRiskReleases()
	if err != nil {
		risks = append(risks, "other Helm releases could not be checked: "+err.Error())
	}
	for _, other := range releases {
		if other.Name == releaseName && other.Namespace == namespace {
			continue
		}
		otherCRDs, err := ChartCRDs(other.Chart)
		if err != nil {
			risks = append(risks, "Helm release "+other.Namespace+"/"+other.Name+" could not be checked: "+err.Error())
			continue
		}
		for name := range otherCRDs {
			if selected[name] {
				risks = append(risks, name+" is used by Helm release "+other.Namespace+"/"+other.Name)
			}
		}
	}
	risks = append(risks, customResourceRisks(ctx, h.sdk.DynamicClient(), names, releaseName, namespace)...)
	sort.Strings(risks)
	return risks
}

func (h *Helm) listCRDRiskReleases() ([]*release.Release, error) {
	cfg, err := h.getConfiguration("")
	if err != nil {
		return nil, err
	}
	list := action.NewList(cfg)
	list.AllNamespaces = true
	list.Deployed = true
	list.Failed = true
	list.Pending = true
	list.Uninstalling = true
	return list.Run()
}

func customResourceRisks(ctx context.Context, client dynamic.Interface, names []string, releaseName, namespace string) []string {
	risks := []string{}
	for _, name := range names {
		crd, err := client.Resource(crdResource).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			risks = append(risks, name+" live definition cannot be checked: "+err.Error())
			continue
		}
		group, _, _ := unstructured.NestedString(crd.Object, "spec", "group")
		plural, _, _ := unstructured.NestedString(crd.Object, "spec", "names", "plural")
		versions, _, _ := unstructured.NestedSlice(crd.Object, "spec", "versions")
		version := ""
		for _, raw := range versions {
			item, ok := raw.(map[string]interface{})
			if ok && item["served"] == true {
				version, _ = item["name"].(string)
				break
			}
		}
		if group == "" || plural == "" || version == "" {
			risks = append(risks, name+" has no readable served resource version")
			continue
		}
		items, err := client.Resource(schema.GroupVersionResource{Group: group, Version: version, Resource: plural}).List(ctx, metav1.ListOptions{})
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			risks = append(risks, name+" custom resources cannot be checked: "+err.Error())
			continue
		}
		for _, item := range items.Items {
			if !customResourceOwnedByRelease(item, releaseName, namespace) {
				risks = append(risks, name+" has custom resources that may belong to another application")
				break
			}
		}
	}
	return risks
}

func customResourceOwnedByRelease(item unstructured.Unstructured, releaseName, namespace string) bool {
	return item.GetLabels()["w7.cc/group-name"] == releaseName && item.GetNamespace() == namespace
}
