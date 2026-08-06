package appgroup

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/w7panel/w7panel/common/service/k8s"
)

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

func TestDownStatic(t *testing.T) {

	os.Setenv("MICROAPP_PATH", "/home/workspace/w7panel/kodata/microapp")
	fetchWebZipAndDownload("http://zpk.w7.cc/zpk/respo/info/w7_zpkv2", "w7-zpkv2", "2.1.67")
	// kName = "w7_sitemanager"
	// cacheKey := staticDownloadCacheKey + kName + "" + version
}

func TestDownGroup(t *testing.T) {
	os.Setenv("STATIC_DOWN_ENABLED", "true")
	os.Setenv("MICROAPP_PATH", "/home/workspace/w7panel/kodata/microapp")
	appgroupObj, err := GetAppgroupUseSdk("w7-sitemanager-ipjjizit", "default", k8s.NewK8sClient().Sdk)
	if err != nil {
		t.Error(err)
	}
	DownStatic(appgroupObj)
	DownStaticStatus("w7-sitemanager", "1.0.25", "w7-sitemanager-ipjjizit")
}
