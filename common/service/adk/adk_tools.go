package adk

type kubectlCommandArgs struct {
	Command string `json:"command" jsonschema:"A kubectl command without shell operators"`
}
type kubectlResult struct {
	ID        string `json:"id"`
	Operation string `json:"operation"`
	Resource  string `json:"resource"`
}

// func proxyTool() (tool.Tool, error) {
// 	proxyTool, err := functiontool.New(functiontool.Config{Name: "k8s_proxy_request", Description: "Make a read-only GET request through the current user's Kubernetes proxy credential."}, func(_ agent.Context, args copilotProxyArgs) (copilotProxyResult, error) {
// 		return copilotProxyGet(requestCtx, token, args.Path)
// 	})
// 	if err != nil {
// 		return nil, err
// 	}
// }

// func kubectlTool() (tool.Tool, error) {
// 	kubectlTool, err := functiontool.New(functiontool.Config{
// 		Name:        "bash_kubectl",
// 		Description: "Propose a kubectl command for user confirmation; it cannot execute until the user confirms."},
// 		func(actx agent.Context, args kubectlCommandArgs) (kubectlResult, error) {

// 		})

// 	return kubectlTool, err
// }

// func copilotTools(requestCtx context.Context, token, actor, namespace string) ([]tool.Tool, error) {
// 	contextTool, err := functiontool.New(functiontool.Config{Name: "get_cluster_context", Description: "Get permitted node, pod, event, and metric summaries for the current namespace."}, func(_ agent.Context, _ copilotNoArgs) (copilotContext, error) {
// 		return loadCopilotContext(requestCtx, token, namespace, false)
// 	})
// 	if err != nil {
// 		return nil, err
// 	}
// 	logsTool, err := functiontool.New(functiontool.Config{Name: "get_pod_logs", Description: "Get the most recent 100 lines from one named Pod container in the current namespace."}, func(_ agent.Context, args copilotPodLogsArgs) (copilotPodLogsResult, error) {
// 		if args.Pod == "" || args.Container == "" {
// 			return copilotPodLogsResult{}, fmt.Errorf("pod and container are required")
// 		}
// 		sdk, err := k8s.NewK8sClient().Channel(token)
// 		if err != nil {
// 			return copilotPodLogsResult{}, err
// 		}
// 		tailLines := int64(100)
// 		content, err := sdk.ClientSet.CoreV1().Pods(namespaceOrDefault(namespace, sdk.GetNamespace())).GetLogs(args.Pod, &corev1.PodLogOptions{Container: args.Container, TailLines: &tailLines}).DoRaw(requestCtx)
// 		if err != nil {
// 			return copilotPodLogsResult{}, err
// 		}
// 		return copilotPodLogsResult{Pod: args.Pod, Container: args.Container, Content: string(content)}, nil
// 	})
// 	if err != nil {
// 		return nil, err
// 	}
// 	proxyTool, err := functiontool.New(functiontool.Config{Name: "k8s_proxy_request", Description: "Make a read-only GET request through the current user's Kubernetes proxy credential."}, func(_ agent.Context, args copilotProxyArgs) (copilotProxyResult, error) {
// 		return copilotProxyGet(requestCtx, token, args.Path)
// 	})
// 	if err != nil {
// 		return nil, err
// 	}
// 	changeTool, err := functiontool.New(functiontool.Config{Name: "propose_resource_change", Description: "Dry-run exactly one non-Secret Kubernetes manifest and create a user-confirmed change proposal."}, func(_ agent.Context, args copilotChangeArgs) (copilotChangeResult, error) {
// 		proposal, err := createCopilotProposal(requestCtx, token, actor, args.Operation, args.Manifest)
// 		if err != nil {
// 			return copilotChangeResult{}, err
// 		}
// 		return copilotChangeResult{ID: proposal.ID, Operation: proposal.Operation, Resource: proposal.Resource}, nil
// 	})
// 	if err != nil {
// 		return nil, err
// 	}
// 	kubectlTool, err := functiontool.New(functiontool.Config{Name: "bash_kubectl", Description: "Propose a kubectl command for user confirmation; it cannot execute until the user confirms."}, func(_ agent.Context, args copilotKubectlCommandArgs) (copilotChangeResult, error) {
// 		proposal, err := createCopilotKubectlProposal(actor, args.Command)
// 		if err != nil {
// 			return copilotChangeResult{}, err
// 		}
// 		return copilotChangeResult{ID: proposal.ID, Operation: proposal.Operation, Resource: proposal.Resource}, nil
// 	})
// 	if err != nil {
// 		return nil, err
// 	}
// 	return []tool.Tool{contextTool, logsTool, proxyTool, changeTool, kubectlTool}, nil
// }

// func createCopilotKubectlProposal(actor, command string) (copilotProposalResponse, error) {
// 	args, err := copilotKubectlArgs(command)
// 	if err != nil {
// 		return copilotProposalResponse{}, err
// 	}
// 	id, err := newCopilotProposalID()
// 	if err != nil {
// 		return copilotProposalResponse{}, err
// 	}
// 	expiresAt := time.Now().Add(10 * time.Minute)
// 	copilotProposals.Lock()
// 	copilotProposals.items[id] = copilotProposal{Actor: actor, Operation: "command", Command: args, ExpiresAt: expiresAt}
// 	copilotProposals.Unlock()
// 	return copilotProposalResponse{ID: id, Operation: "command", Resource: strings.Join(args, " "), ExpiresAt: expiresAt}, nil
// }

// func copilotKubectlArgs(command string) ([]string, error) {
// 	if strings.ContainsAny(command, "\n\r;|&><`$") {
// 		return nil, fmt.Errorf("shell operators are not allowed")
// 	}
// 	args := strings.Fields(command)
// 	if len(args) < 2 || len(args) > 32 || args[0] != "kubectl" {
// 		return nil, fmt.Errorf("command must be a kubectl command with at most 31 arguments")
// 	}
// 	for _, arg := range args[1:] {
// 		lower := strings.ToLower(arg)
// 		if lower == "secret" || lower == "secrets" || strings.HasPrefix(lower, "--kubeconfig") || strings.HasPrefix(lower, "--server") || strings.HasPrefix(lower, "--token") || strings.HasPrefix(lower, "--context") {
// 			return nil, fmt.Errorf("command may not access Secrets or override cluster credentials")
// 		}
// 	}
// 	return args, nil
// }

// func runCopilotKubectl(ctx context.Context, args []string) (string, error) {
// 	commandCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
// 	defer cancel()
// 	output, err := exec.CommandContext(commandCtx, args[0], args[1:]...).CombinedOutput()
// 	return string(output), err
// }

// func createCopilotProposal(ctx context.Context, token, actor, operation, manifest string) (copilotProposalResponse, error) {
// 	if operation == "" {
// 		operation = "apply"
// 	}
// 	if operation != "apply" && operation != "delete" {
// 		return copilotProposalResponse{}, fmt.Errorf("operation must be apply or delete")
// 	}
// 	object, err := decodeCopilotObject(manifest)
// 	if err != nil {
// 		return copilotProposalResponse{}, err
// 	}
// 	if strings.EqualFold(object.GetKind(), "Secret") {
// 		return copilotProposalResponse{}, fmt.Errorf("Copilot does not handle Secret resources")
// 	}
// 	if err := dryRunCopilotAction(ctx, token, operation, object); err != nil {
// 		return copilotProposalResponse{}, fmt.Errorf("resource proposal is not permitted: %w", err)
// 	}
// 	id, err := newCopilotProposalID()
// 	if err != nil {
// 		return copilotProposalResponse{}, err
// 	}
// 	expiresAt := time.Now().Add(10 * time.Minute)
// 	copilotProposals.Lock()
// 	copilotProposals.items[id] = copilotProposal{Actor: actor, Operation: operation, Object: object, ExpiresAt: expiresAt}
// 	copilotProposals.Unlock()
// 	return copilotProposalResponse{ID: id, Operation: operation, Resource: resourceRef(object), ExpiresAt: expiresAt}, nil
// }
