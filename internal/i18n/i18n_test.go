package i18n

import "testing"

func TestNormalize(t *testing.T) {
	cases := map[string]Lang{
		"en": EN, "EN": EN, "English": EN, "en-US": EN,
		"ja": JA, "jp": JA, "Japanese": JA,
		"": JA, "xx": JA, "  ja  ": JA,
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q)=%q want %q", in, got, want)
		}
	}
}

func TestTFallback(t *testing.T) {
	// 既知キーは各言語で引ける。
	if got := T(JA, "saved"); got != "-- 保存しました --" {
		t.Errorf("ja saved = %q", got)
	}
	if got := T(EN, "saved"); got != "-- saved --" {
		t.Errorf("en saved = %q", got)
	}
	// 空言語は Default(ja) にフォールバック。
	if got := T("", "saved"); got != "-- 保存しました --" {
		t.Errorf("empty saved = %q", got)
	}
	// 未知キーはキー自身。
	if got := T(EN, "no.such.key"); got != "no.such.key" {
		t.Errorf("unknown key = %q", got)
	}
}

func TestTArgs(t *testing.T) {
	if got := T(EN, "lang.saved", "English"); got != "-- language set to English --" {
		t.Errorf("args = %q", got)
	}
	// 未知キー＋引数は Sprintf でキーを書式として使う（壊れない）。
	if got := T(EN, "%s!", "hi"); got != "hi!" {
		t.Errorf("unknown key with args = %q", got)
	}
}
