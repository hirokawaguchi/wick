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

// ReadLang は言語別アセットを読む。まず <root>/<lang>/<elem...> を探し、
// 無ければ基準アセット <root>/<elem...>（＝ja 相当）へフォールバックする。
// lang が空や既定(ja)のときは基準アセットをそのまま読む。
func (d Dir) ReadLang(lang string, elem ...string) (string, error) {
	if lang != "" && lang != "ja" {
		if d.Exists(append([]string{lang}, elem...)...) {
			return d.Read(append([]string{lang}, elem...)...)
		}
	}
	return d.Read(elem...)
}

// ExistsLang は言語別アセットまたは基準アセットが在るか。
func (d Dir) ExistsLang(lang string, elem ...string) bool {
	if lang != "" && lang != "ja" && d.Exists(append([]string{lang}, elem...)...) {
		return true
	}
	return d.Exists(elem...)
}
