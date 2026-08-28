package middleware

import (
	"os"
	"strings"
)

const (
	PanelAuthMode = "panel"
	K8sAuthMode   = "k8s"
)

// ckmAuthMode selects how requests originating from CKM are authenticated.
// Keep k8s as the default for backwards compatibility with child clusters.
func ckmAuthMode() string {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("W7PANEL_AUTH_MODE")))
	if mode != PanelAuthMode && mode != K8sAuthMode {
		return K8sAuthMode
	}
	return mode
}

func panelTokenHeaderEnabled() bool {
	return ckmAuthMode() == PanelAuthMode
}
