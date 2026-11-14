//go:build darwin

package i18n

import (
	"os/exec"
	"strings"
	"sync"
)

var (
	osLangOnce sync.Once
	osLang     string
)

// osLanguage reads the preferred language from macOS. An app launched from the
// Finder inherits no LANG/LC_ALL, so without this "system" always fell back to
// English even when the Mac's UI language was Chinese.
func osLanguage() string {
	osLangOnce.Do(func() {
		out, err := exec.Command("defaults", "read", "-g", "AppleLanguages").Output()
		if err != nil {
			return
		}
		for _, tag := range strings.FieldsFunc(string(out), func(r rune) bool {
			switch r {
			case ',', '(', ')', '"', '\'', ' ', '\n', '\r', '\t':
				return true
			}
			return false
		}) {
			if lang := matchLanguage(tag); lang != "" {
				osLang = lang
				return
			}
		}
	})
	return osLang
}
