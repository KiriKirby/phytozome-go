package locale

import "testing"

func TestParseLanguage(t *testing.T) {
	cases := []struct {
		input string
		want  Language
		ok    bool
	}{
		{"en", English, true},
		{"cn", Chinese, true},
		{"jp", Japanese, true},
		{"english", English, true},
		{"中文", Chinese, true},
		{"日本語", Japanese, true},
		{"unknown", English, false},
	}

	for _, tc := range cases {
		got, ok := Parse(tc.input)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("Parse(%q) = (%v, %v), want (%v, %v)", tc.input, got, ok, tc.want, tc.ok)
		}
	}
}

func TestDetectFromExecutable(t *testing.T) {
	cases := map[string]Language{
		"phytozome-go-cn.exe": Chinese,
		"phytozome-go-jp":     Japanese,
		"phytozome-go-en.exe": English,
		"phytozome-go":        English,
	}
	for input, want := range cases {
		if got := DetectFromExecutable(input); got != want {
			t.Fatalf("DetectFromExecutable(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestDetectFromArgs(t *testing.T) {
	lang, found, kept := DetectFromArgs([]string{"blast", "lang=cn", "plan"})
	if !found || lang != Chinese {
		t.Fatalf("DetectFromArgs did not detect cn: lang=%v found=%v", lang, found)
	}
	if len(kept) != 2 || kept[0] != "blast" || kept[1] != "plan" {
		t.Fatalf("DetectFromArgs kept wrong args: %v", kept)
	}
}
