package i18n

import "testing"

func TestResolve(t *testing.T) {
	cases := map[string]string{
		"zh":       "zh",
		"zh-CN":    "zh",
		" ZH-Hans": "zh",
		"zh-TW":    "zh",
		"en":       "en",
		"en-US":    "en",
		"fr":       "en",
	}
	for in, want := range cases {
		if got := Resolve(in); got != want {
			t.Errorf("Resolve(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveSystemFollowsEnvironment(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")

	t.Setenv("LANG", "zh_CN.UTF-8")
	if got := Resolve("system"); got != "zh" {
		t.Fatalf("system with LANG=zh_CN.UTF-8 = %q, want zh", got)
	}

	t.Setenv("LANG", "en_US.UTF-8")
	if got := Resolve("system"); got != "en" {
		t.Fatalf("system with LANG=en_US.UTF-8 = %q, want en", got)
	}
}

func TestBundleSetLanguageSwapsDictionary(t *testing.T) {
	b := New("en")
	if got := b.T("settings.languageSystem"); got != "System" {
		t.Fatalf("en dictionary: %q", got)
	}
	b.SetLanguage("zh-CN")
	if got := b.Language(); got != "zh" {
		t.Fatalf("Language() = %q, want zh", got)
	}
	if got := b.T("settings.languageSystem"); got != "跟随系统" {
		t.Fatalf("zh dictionary: %q", got)
	}
}
