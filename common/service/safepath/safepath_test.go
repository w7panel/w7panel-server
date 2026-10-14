package safepath

import "testing"

func TestResolveRejectsTraversal(t *testing.T) {
	for _, input := range []string{"../secret", "/etc/passwd", "a/../../secret", "a\\..\\..\\secret", "\x00x"} {
		if _, err := Resolve(t.TempDir(), input); err == nil {
			t.Fatalf("Resolve(%q) unexpectedly succeeded", input)
		}
	}
}

func TestResolveKeepsRelativePathInRoot(t *testing.T) {
	root := t.TempDir()
	if got, err := Resolve(root, "upload/source.zip"); err != nil || got == root {
		t.Fatalf("Resolve() = %q, %v", got, err)
	}
}
