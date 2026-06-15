package controller

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveMicroappLocalFileReturnsExistingFile(t *testing.T) {
	root := t.TempDir()
	htmlFile := filepath.Join(root, "w7panel-ckm", "1.1.28", "123.html")
	if err := os.MkdirAll(filepath.Dir(htmlFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(htmlFile, []byte("<html></html>"), 0644); err != nil {
		t.Fatal(err)
	}

	got, found, err := resolveMicroappLocalFile(root, "w7panel-ckm", "1.1.28", "/123.html")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected existing file to be found")
	}
	if got != htmlFile {
		t.Fatalf("expected %q, got %q", htmlFile, got)
	}
}

func TestResolveMicroappLocalFileReturnsMissingHTMLPage(t *testing.T) {
	root := t.TempDir()
	indexFile := filepath.Join(root, "w7panel-ckm", "1.1.28", "index.html")
	if err := os.MkdirAll(filepath.Dir(indexFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexFile, []byte("<html></html>"), 0644); err != nil {
		t.Fatal(err)
	}

	_, found, err := resolveMicroappLocalFile(root, "w7panel-ckm", "1.1.28", "/123.html")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("expected missing html page to return not found")
	}
}

func TestResolveMicroappLocalFileReturnsMissingStaticAsset(t *testing.T) {
	root := t.TempDir()
	indexFile := filepath.Join(root, "w7panel-ckm", "1.1.28", "index.html")
	if err := os.MkdirAll(filepath.Dir(indexFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexFile, []byte("<html></html>"), 0644); err != nil {
		t.Fatal(err)
	}

	_, found, err := resolveMicroappLocalFile(root, "w7panel-ckm", "1.1.28", "/assets/missing.js")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("expected missing static asset to return not found")
	}
}

func TestResolveMicroappLocalFileRejectsParentDirectory(t *testing.T) {
	_, _, err := resolveMicroappLocalFile(t.TempDir(), "w7panel-ckm", "1.1.28", "/../secret.html")
	if err == nil {
		t.Fatal("expected parent directory path to be rejected")
	}
}
