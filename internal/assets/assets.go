package assets

import (
	"os"
	"path/filepath"
	"strings"
)

type Dir struct {
	Root string
}

func (d Dir) Path(elem ...string) string {
	return filepath.Join(append([]string{d.Root}, elem...)...)
}

func (d Dir) Read(elem ...string) (string, error) {
	b, err := os.ReadFile(d.Path(elem...))
	if err != nil {
		return "", err
	}
	s := string(b)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.TrimRight(s, "\n") + "\n", nil
}

func (d Dir) Exists(elem ...string) bool {
	_, err := os.Stat(d.Path(elem...))
	return err == nil
}
