package logic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/w7panel/w7panel/common/service/k8s/zpk/logic/types"
)

func isCKMChildPanelUpgrade(params InstallRequest, pack *types.ManifestPackage) bool {
	return os.Getenv("IS_CHILD") == "true" && params.Namespace == "default" &&
		params.ReleaseName == os.Getenv("SVC_NAME") &&
		pack.Manifest.Application.Identifie == "w7panel-offline" &&
		strings.HasPrefix(params.RepoUrl, "https://zpk.w7.cc/zpk/respo/info/w7panel")
}

func requestCKMChildPanelUpgrade(ctx context.Context, releaseName, version string) error {
	endpoint := strings.TrimRight(os.Getenv("CKM_SYNC_ENDPOINT"), "/")
	token := os.Getenv("CKM_SYNC_TOKEN")
	if endpoint == "" || token == "" || os.Getenv("CKM_NAME") == "" || os.Getenv("K3K_NAME") == "" || os.Getenv("K3K_NAMESPACE") == "" {
		return fmt.Errorf("CKM child panel upgrade channel is unavailable")
	}
	payload, err := json.Marshal(map[string]string{
		"ckmName": os.Getenv("CKM_NAME"), "k3kName": os.Getenv("K3K_NAME"),
		"k3kNamespace": os.Getenv("K3K_NAMESPACE"), "releaseName": releaseName, "version": version,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/child-panel-chart", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-W7Panel-CKM-Sync-Token", token)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("CKM child panel upgrade HTTP %d: %s", resp.StatusCode, message)
	}
	return nil
}
