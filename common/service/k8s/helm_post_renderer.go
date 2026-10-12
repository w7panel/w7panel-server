package k8s

import (
	"bytes"
	"context"
	"fmt"

	"helm.sh/helm/v3/pkg/postrender"
)

// helmPostRenderer runs registered renderers in order, passing each result to
// the next renderer. An error stops the pipeline before Helm applies resources.
type helmPostRenderer struct {
	renderers []postrender.PostRenderer
}

func (r *helmPostRenderer) Run(input *bytes.Buffer) (*bytes.Buffer, error) {
	output := input
	for _, renderer := range r.renderers {
		var err error
		output, err = renderer.Run(output)
		if err != nil {
			return nil, fmt.Errorf("Helm post-renderer %T: %w", renderer, err)
		}
		if output == nil {
			return nil, fmt.Errorf("Helm post-renderer %T returned no manifest", renderer)
		}
	}
	return output, nil
}

// newUpgradePostRenderer is the registration point for upgrade renderers.
func (h *Helm) newUpgradePostRenderer(ctx context.Context, namespace string) postrender.PostRenderer {
	return &helmPostRenderer{renderers: []postrender.PostRenderer{
		&preservePVCStorageRenderer{ctx: ctx, client: h.sdk.ClientSet.CoreV1(), namespace: namespace},
	}}
}
