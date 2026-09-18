// Package safepath provides filesystem containment checks for paths supplied
// by panel clients. filepath.Join alone is insufficient because an absolute
// path or a parent segment discards/escapes the intended root.
package safepath

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

func Identifier(value string) error {
	if !identifierPattern.MatchString(value) {
		return errors.New("invalid file identifier")
	}
	return nil
}

// Resolve returns an absolute target within root. Symlink traversal is not
// accepted for existing parents, so a writable upload directory cannot point
// an otherwise-valid relative name outside its root.
func Resolve(root, userPath string) (string, error) {
	if userPath == "" || strings.ContainsRune(userPath, 0) || filepath.IsAbs(userPath) {
		return "", errors.New("path must be a non-empty relative path")
	}
	for _, part := range strings.FieldsFunc(strings.ReplaceAll(userPath, "\\", "/"), func(r rune) bool { return r == '/' }) {
		if part == ".." {
			return "", errors.New("path contains parent directory")
		}
	}
	base, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(filepath.Join(base, userPath))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes root")
	}
	return target, nil
}
