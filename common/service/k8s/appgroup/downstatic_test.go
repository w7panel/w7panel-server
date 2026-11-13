package appgroup

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/w7panel/w7panel/common/service/k8s"
)

func testFrontendZip(t *testing.T) []byte {
	t.Helper()
	var content bytes.Buffer
	writer := zip.NewWriter(&content)
	file, err := writer.Create("index.html")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("<html>cached</html>")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return content.Bytes()
}

func TestDownStaticMapReusesCompleteHostCache(t *testing.T) {
	root := t.TempDir()
	archive := testFrontendZip(t)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	for range 2 {
		if err := downStaticMap(map[string]string{"cache-app": server.URL}, nil, "cache-release", root, "1.0.0"); err != nil {
			t.Fatal(err)
		}
	}
	if requests != 1 {
		t.Fatalf("ZIP downloads = %d, want 1", requests)
	}
	if !hasCompleteMarker(root, "cache-app", "1.0.0") || !hasCompleteMarker(root, "cache-release") {
		t.Fatal("completed cache markers are missing")
	}
	if _, err := os.Stat(filepath.Join(root, "cache-app", "1.0.0", "index.html")); err != nil {
		t.Fatal(err)
	}
	if err := downStaticMap(map[string]string{"cache-app": server.URL}, nil, "cache-release", root, "2.0.0"); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("new version ZIP downloads = %d, want 2", requests)
	}
}

func TestDownStaticMapRetriesIncompleteCache(t *testing.T) {
	root := t.TempDir()
	archive := testFrontendZip(t)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			_, _ = w.Write([]byte("not a zip"))
			return
		}
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	files := map[string]string{"cache-app": server.URL}
	if err := downStaticMap(files, nil, "cache-release", root, "1.0.0"); err == nil {
		t.Fatal("expected invalid ZIP to fail")
	}
	if hasCompleteMarker(root, "cache-app", "1.0.0") || hasCompleteMarker(root, "cache-release") {
		t.Fatal("incomplete cache was marked complete")
	}
	if err := downStaticMap(files, nil, "cache-release", root, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("ZIP downloads = %d, want 2", requests)
	}
}

func TestDownStaticStatusReadsCompletedDiskCache(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MICROAPP_PATH", root)
	identifie := fmt.Sprintf("cache-app-%s", filepath.Base(root))
	version := "1.0.0"
	if got := DownStaticStatus(identifie, version, ""); got != NO_DOWN {
		t.Fatalf("missing cache status = %q, want %q", got, NO_DOWN)
	}
	if err := os.MkdirAll(filepath.Join(root, identifie, version), 0755); err != nil {
		t.Fatal(err)
	}
	if err := writeCompleteMarker(root, identifie, version); err != nil {
		t.Fatal(err)
	}
	if got := DownStaticStatus(identifie, version, ""); got != DOWNLOAD_SUCCESS {
		t.Fatalf("persisted cache status = %q, want %q", got, DOWNLOAD_SUCCESS)
	}
	releaseName := identifie + "-release"
	if err := os.MkdirAll(filepath.Join(root, releaseName), 0755); err != nil {
		t.Fatal(err)
	}
	if err := writeCompleteMarker(root, releaseName); err != nil {
		t.Fatal(err)
	}
	if got := DownStaticStatus("", "", releaseName); got != DOWNLOAD_SUCCESS {
		t.Fatalf("persisted release status = %q, want %q", got, DOWNLOAD_SUCCESS)
	}
}

func TestExtractZipToDirRejectsPathTraversal(t *testing.T) {
	tempDir := t.TempDir()
	zipPath := filepath.Join(tempDir, "malicious.zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(zipFile)
	entry, err := writer.Create("../escaped.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("unexpected")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zipFile.Close(); err != nil {
		t.Fatal(err)
	}

	destDir := filepath.Join(tempDir, "destination")
	if err := extractZipToDir(zipPath, destDir); err == nil {
		t.Fatal("expected path traversal archive to be rejected")
	}
	if _, err := os.Stat(filepath.Join(tempDir, "escaped.txt")); !os.IsNotExist(err) {
		t.Fatalf("archive wrote outside the destination, err=%v", err)
	}
}

func disabledDownStatic(t *testing.T) {

	os.Setenv("MICROAPP_PATH", "/home/workspace/w7panel/kodata/microapp")
	fetchWebZipAndDownload("http://zpk.w7.cc/zpk/respo/info/w7_zpkv2", "w7-zpkv2", "2.1.67", nil, "")
	// kName = "w7_sitemanager"
	// cacheKey := staticDownloadCacheKey + kName + "" + version
}

func disabledDownGroup(t *testing.T) {
	os.Setenv("STATIC_DOWN_ENABLED", "true")
	os.Setenv("MICROAPP_PATH", "/home/workspace/w7panel/kodata/microapp")
	appgroupObj, err := GetAppgroupUseSdk("w7-sitemanager-ipjjizit", "default", k8s.NewK8sClient().Sdk)
	if err != nil {
		t.Error(err)
	}
	DownStatic(appgroupObj, nil, "")
	DownStaticStatus("w7-sitemanager", "1.0.25", "w7-sitemanager-ipjjizit")
}
