// Package i18n carries the native UI's translations. The dictionaries are the
// same JSON files the former React frontend used, so keys and copy stay in
// step while the web layer is removed.
package i18n

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

//go:embed en.json
var enJSON []byte

//go:embed zh.json
var zhJSON []byte

// Bundle is a loaded set of translations.
type Bundle struct {
	mu   sync.RWMutex
	lang string
	data map[string]any
}

// New returns a bundle for the given language preference ("system", "en",
// "zh", "zh-CN"). An unknown preference falls back to en.
func New(language string) *Bundle {
	b := &Bundle{}
	b.SetLanguage(language)
	return b
}

// SetLanguage replaces the active dictionary.
func (b *Bundle) SetLanguage(language string) {
	lang := Resolve(language)
	var raw []byte
	switch lang {
	case "zh":
		raw = zhJSON
	default:
		raw = enJSON
	}
	data := map[string]any{}
	_ = json.Unmarshal(raw, &data)
	b.mu.Lock()
	b.lang = lang
	b.data = data
	b.mu.Unlock()
}

// Language returns the resolved language code.
func (b *Bundle) Language() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.lang
}

// Resolve maps a stored preference to a concrete language.
func Resolve(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "zh", "zh-cn", "zh-hans", "zh-hant", "zh-tw":
		return "zh"
	case "en", "en-us", "en-gb":
		return "en"
	case "", "system":
		return systemLanguage()
	default:
		return "en"
	}
}

// systemLanguage follows the desktop's UI language. Environment variables come
// first (they are what a terminal launch sets and what tests can override),
// then the platform lookup for apps started from the Finder or Explorer.
func systemLanguage() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if lang := matchLanguage(os.Getenv(key)); lang != "" {
			return lang
		}
	}
	if lang := osLanguage(); lang != "" {
		return lang
	}
	return "en"
}

// matchLanguage maps a locale tag such as "zh_CN.UTF-8" or "en-US" to a
// dictionary. Empty means "not a language we ship", which keeps the caller
// falling through to the next source instead of always resolving to English.
func matchLanguage(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	switch {
	case strings.HasPrefix(tag, "zh"):
		return "zh"
	case strings.HasPrefix(tag, "en"):
		return "en"
	default:
		return ""
	}
}

// T translates key. A missing key is returned unchanged, which keeps layout
// and diagnostics readable when a new string has not been translated yet.
func (b *Bundle) T(key string) string {
	return b.Tf(key, nil)
}

// Tf translates key and substitutes {{name}} placeholders.
func (b *Bundle) Tf(key string, args map[string]any) string {
	b.mu.RLock()
	value := lookup(b.data, key)
	b.mu.RUnlock()
	if value == "" {
		return key
	}
	for name, arg := range args {
		value = strings.ReplaceAll(value, "{{"+name+"}}", fmt.Sprint(arg))
	}
	return value
}

func lookup(data map[string]any, key string) string {
	var current any = data
	for _, part := range strings.Split(key, ".") {
		m, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current, ok = m[part]
		if !ok {
			return ""
		}
	}
	s, _ := current.(string)
	return s
}
