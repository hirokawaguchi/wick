package testenv

import (
	"os"
	"path/filepath"
	"testing"
)

func Root(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		root := filepath.Join(dir, "data")
		if _, err := os.Stat(filepath.Join(root, "etc", "COMMAND.TXT")); err == nil {
			return root
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}
	t.Fatal("data/etc/COMMAND.TXT not found")
	return ""
}
