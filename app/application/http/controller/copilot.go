package controller

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
)

const copilotSystemPrompt = `You are W7Panel Copilot. Diagnose Kubernetes from supplied context without inventing data. Never request or expose Secret values. Output only OpenUI Lang statements. The first statement must be root = CopilotCard([children], "title"). Components are CopilotCard(children, title), CopilotText(text), CopilotMetric(label, value), CopilotAlert(text, level), and CopilotYaml(operation, manifest). Every non-root variable must be referenced by its parent. Use CopilotYaml only when the user explicitly asks for a change; explain impact with CopilotAlert before it. Its operation is "apply" or "delete" and manifest is exactly one Kubernetes YAML manifest. Do not claim a change has been applied.`

type Copilot struct{ controller.Abstract }

type copilotMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type copilotStreamRequest struct {
	Messages  []copilotMessage `json:"messages"`
	Namespace string           `json:"namespace"`
}

type copilotProposal struct {
	Actor     string
	Operation string
	Object    *unstructured.Unstructured
	ExpiresAt time.Time
}

var copilotProposals = struct {
	sync.Mutex
	items map[string]copilotProposal
}{items: map[string]copilotProposal{}}

func (Copilot) Stream(ctx *gin.Context) {
	request := copilotStreamRequest{}
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"msg": "invalid Copilot request"})
		return
	}
	if len(request.Messages) == 0 || len(request.Messages) > 40 {
		ctx.JSON(http.StatusBadRequest, gin.H{"msg": "messages must contain 1 to 40 items"})
		return
	}
	messages := make([]map[string]string, 0, len(request.Messages)+1)
	messages = append(messages, map[string]string{"role": "system", "content": copilotSystemPrompt})
	for _, message := range request.Messages {
		if (message.Role != "user" && message.Role != "assistant") || len(message.Content) == 0 || len(message.Content) > 16000 {
			ctx.JSON(http.StatusBadRequest, gin.H{"msg": "invalid Copilot message"})
			return
		}
		messages = append(messages, map[string]string{"role": message.Role, "content": message.Content})
	}

	baseURL := strings.TrimRight(facade.Config.GetString("copilot.openai_base_url"), "/")
	apiKey := facade.Config.GetString("copilot.openai_api_key")
	model := facade.Config.GetString("copilot.model")
	if !facade.Config.GetBool("copilot.enabled") || baseURL == "" || apiKey == "" || model == "" {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"msg": "Copilot is not configured"})
		return
	}
	body, _ := json.Marshal(gin.H{"model": model, "stream": true, "messages": messages})
	upstream, err := http.NewRequestWithContext(ctx.Request.Context(), http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		ctx.JSON(http.StatusBadGateway, gin.H{"msg": "invalid Copilot model endpoint"})
		return
	}
	upstream.Header.Set("Authorization", "Bearer "+apiKey)
	upstream.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 90 * time.Second}).Do(upstream)
	if err != nil {
		ctx.JSON(http.StatusBadGateway, gin.H{"msg": "Copilot model request failed"})
		return
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusMultipleChoices {
		ctx.JSON(http.StatusBadGateway, gin.H{"msg": "Copilot model returned an error"})
		return
	}
	ctx.Header("Content-Type", "text/event-stream")
	ctx.Header("Cache-Control", "no-cache")
	ctx.Status(http.StatusOK)
	if flusher, ok := ctx.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
	_, _ = io.Copy(ctx.Writer, response.Body)
}

func (Copilot) Context(ctx *gin.Context) {
	sdk, err := k8s.NewK8sClient().Channel(ctx.MustGet("k8s_token").(string))
	if err != nil {
		ctx.JSON(http.StatusBadGateway, gin.H{"msg": "unable to query cluster"})
		return
	}
	namespace := ctx.Query("namespace")
	if namespace == "" {
		namespace = sdk.GetNamespace()
	}
	nodes, err := sdk.ClientSet.CoreV1().Nodes().List(ctx.Request.Context(), metav1.ListOptions{})
	if err != nil {
		self := Copilot{}
		self.JsonResponseWithServerError(ctx, err)
		return
	}
	pods, err := sdk.ClientSet.CoreV1().Pods(namespace).List(ctx.Request.Context(), metav1.ListOptions{})
	if err != nil {
		self := Copilot{}
		self.JsonResponseWithServerError(ctx, err)
		return
	}
	events, err := sdk.ClientSet.CoreV1().Events(namespace).List(ctx.Request.Context(), metav1.ListOptions{Limit: 20})
	if err != nil {
		self := Copilot{}
		self.JsonResponseWithServerError(ctx, err)
		return
	}
	eventSummary := make([]gin.H, 0, len(events.Items))
	for _, event := range events.Items {
		eventSummary = append(eventSummary, gin.H{"reason": event.Reason, "message": event.Message, "type": event.Type, "object": event.InvolvedObject.Kind + "/" + event.InvolvedObject.Name})
	}
	ctx.JSON(http.StatusOK, gin.H{"namespace": namespace, "nodes": len(nodes.Items), "pods": len(pods.Items), "events": eventSummary})
}

func (Copilot) CreateAction(ctx *gin.Context) {
	var request struct {
		Operation string `json:"operation"`
		Manifest  string `json:"manifest"`
	}
	if err := ctx.ShouldBindJSON(&request); err != nil || len(request.Manifest) == 0 || len(request.Manifest) > 256*1024 {
		ctx.JSON(http.StatusBadRequest, gin.H{"msg": "invalid resource proposal"})
		return
	}
	if request.Operation == "" {
		request.Operation = "apply"
	}
	if request.Operation != "apply" && request.Operation != "delete" {
		ctx.JSON(http.StatusBadRequest, gin.H{"msg": "operation must be apply or delete"})
		return
	}
	object, err := decodeCopilotObject(request.Manifest)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"msg": err.Error()})
		return
	}
	if strings.EqualFold(object.GetKind(), "Secret") {
		ctx.JSON(http.StatusForbidden, gin.H{"msg": "Copilot does not handle Secret resources"})
		return
	}
	if err := dryRunCopilotAction(ctx.Request.Context(), ctx.MustGet("k8s_token").(string), request.Operation, object); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"msg": "resource proposal is not permitted", "detail": err.Error()})
		return
	}
	id, err := newCopilotProposalID()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"msg": "unable to create resource proposal"})
		return
	}
	copilotProposals.Lock()
	copilotProposals.items[id] = copilotProposal{Actor: ctx.GetString("username"), Operation: request.Operation, Object: object, ExpiresAt: time.Now().Add(10 * time.Minute)}
	copilotProposals.Unlock()
	ctx.JSON(http.StatusCreated, gin.H{"id": id, "operation": request.Operation, "resource": resourceRef(object), "expiresAt": time.Now().Add(10 * time.Minute)})
}

func (Copilot) ConfirmAction(ctx *gin.Context) {
	proposal, ok := takeCopilotProposal(ctx.Param("id"), ctx.GetString("username"))
	if !ok {
		ctx.JSON(http.StatusNotFound, gin.H{"msg": "resource proposal was not found or expired"})
		return
	}
	if err := runCopilotAction(ctx.Request.Context(), ctx.MustGet("k8s_token").(string), proposal.Operation, proposal.Object, false); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"msg": "resource change failed", "detail": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"resource": resourceRef(proposal.Object), "operation": proposal.Operation})
}

func (Copilot) RejectAction(ctx *gin.Context) {
	_, _ = takeCopilotProposal(ctx.Param("id"), ctx.GetString("username"))
	ctx.Status(http.StatusNoContent)
}

func decodeCopilotObject(manifest string) (*unstructured.Unstructured, error) {
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 256*1024)
	object := map[string]any{}
	if err := decoder.Decode(&object); err != nil {
		return nil, fmt.Errorf("invalid Kubernetes manifest")
	}
	result := &unstructured.Unstructured{Object: object}
	if result.GetAPIVersion() == "" || result.GetKind() == "" || result.GetName() == "" {
		return nil, fmt.Errorf("manifest requires apiVersion, kind, and metadata.name")
	}
	var extra map[string]any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("only one Kubernetes manifest is allowed")
	}
	return result, nil
}

func dryRunCopilotAction(ctx context.Context, token, operation string, object *unstructured.Unstructured) error {
	return runCopilotAction(ctx, token, operation, object, true)
}

func runCopilotAction(ctx context.Context, token, operation string, object *unstructured.Unstructured, dryRun bool) error {
	sdk, err := k8s.NewK8sClient().Channel(token)
	if err != nil {
		return err
	}
	mapping, err := sdk.GetRestMapping(object.GetAPIVersion(), object.GetKind())
	if err != nil {
		return err
	}
	resource, err := copilotResource(sdk.DynamicClient(), mapping, object.GetNamespace(), sdk.GetNamespace())
	if err != nil {
		return err
	}
	dryRuns := []string(nil)
	if dryRun {
		dryRuns = []string{metav1.DryRunAll}
	}
	if operation == "delete" {
		return resource.Delete(ctx, object.GetName(), metav1.DeleteOptions{DryRun: dryRuns})
	}
	payload, err := json.Marshal(object.Object)
	if err != nil {
		return err
	}
	_, err = resource.Patch(ctx, object.GetName(), types.ApplyPatchType, payload, metav1.PatchOptions{FieldManager: "w7panel-copilot", DryRun: dryRuns})
	return err
}

func copilotResource(client dynamic.Interface, mapping *meta.RESTMapping, requestedNamespace, fallbackNamespace string) (dynamic.ResourceInterface, error) {
	resource := client.Resource(mapping.Resource)
	if mapping.Scope.Name() != meta.RESTScopeNameNamespace {
		return resource, nil
	}
	namespace := requestedNamespace
	if namespace == "" {
		namespace = fallbackNamespace
	}
	if namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}
	return resource.Namespace(namespace), nil
}

func resourceRef(object *unstructured.Unstructured) string {
	gv, _ := schema.ParseGroupVersion(object.GetAPIVersion())
	group := gv.Group
	if group != "" {
		group = "." + group
	}
	if object.GetNamespace() != "" {
		return object.GetKind() + group + "/" + object.GetNamespace() + "/" + object.GetName()
	}
	return object.GetKind() + group + "/" + object.GetName()
}

func newCopilotProposalID() (string, error) {
	bytes := make([]byte, 18)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func takeCopilotProposal(id, actor string) (copilotProposal, bool) {
	copilotProposals.Lock()
	defer copilotProposals.Unlock()
	proposal, ok := copilotProposals.items[id]
	delete(copilotProposals.items, id)
	return proposal, ok && proposal.Actor == actor && time.Now().Before(proposal.ExpiresAt)
}
