// Package query builds the desktop query-string grammar from the mobile
// filter state and describes what the legacy /items endpoint can express.
package query

import (
	"net/url"
	"strconv"
	"strings"
)

// Spec is the structured filter state shared by the gallery and search pages.
// Its zero value means "the whole library".
type Spec struct {
	Text          string
	Type          string
	Orientation   string
	Tag           string
	Folder        string
	Color         string
	Size          string
	Rating        int
	HasURL        string // "", "has" or "empty"
	HasNote       string // "", "has" or "empty"
	Untagged      bool
	Uncategorized bool
}

// String renders the filter state as the wire query string. Free text is
// always quoted so a literal "tag:foo" cannot become a filter.
func (s Spec) String() string {
	parts := make([]string, 0, 12)
	if text := strings.TrimSpace(s.Text); text != "" {
		parts = append(parts, Quote(text))
	}
	if v := strings.TrimSpace(s.Type); v != "" {
		parts = append(parts, "type:"+Quote(v))
	}
	if v := strings.TrimSpace(s.Orientation); v != "" {
		parts = append(parts, "orientation:"+Quote(v))
	}
	if s.Untagged {
		parts = append(parts, "is:untagged")
	}
	if s.Uncategorized {
		parts = append(parts, "is:uncategorized")
	}
	if v := strings.TrimSpace(s.Tag); v != "" {
		parts = append(parts, "tag:"+Quote(v))
	}
	if v := strings.TrimSpace(s.Folder); v != "" {
		parts = append(parts, "folder:"+Quote(v))
	}
	if v := strings.TrimSpace(s.Color); v != "" {
		parts = append(parts, "color:"+Quote(v))
	}
	if v := strings.TrimSpace(s.Size); v != "" {
		parts = append(parts, "size:"+Quote(v))
	}
	if s.Rating > 0 {
		parts = append(parts, "rating:"+strconv.Itoa(s.Rating))
	}
	switch s.HasURL {
	case "has":
		parts = append(parts, "has:url")
	case "empty":
		parts = append(parts, "no:url")
	}
	switch s.HasNote {
	case "has":
		parts = append(parts, "has:note")
	case "empty":
		parts = append(parts, "no:note")
	}
	return strings.Join(parts, " ")
}

// Quote quotes a value when it contains whitespace, a quote, a backslash or a
// colon; plain values stay readable.
func Quote(value string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value)
	if strings.ContainsAny(value, " \t\r\n:\"") {
		return `"` + escaped + `"`
	}
	return escaped
}

// LegacyParams returns the subset of filters GET /api/v1/items understands.
// Callers must also consult Unsupported to tell the user what was dropped.
func (s Spec) LegacyParams() url.Values {
	v := url.Values{}
	set := func(key, value string) {
		if value = strings.TrimSpace(value); value != "" {
			v.Set(key, value)
		}
	}
	set("q", s.Text)
	set("type", s.Type)
	set("orientation", s.Orientation)
	set("tag", s.Tag)
	set("folder", s.Folder)
	set("color", s.Color)
	return v
}

// Unsupported lists the filters the legacy endpoint cannot express. An empty
// result means the fallback is semantically complete.
func (s Spec) Unsupported() []string {
	var out []string
	if strings.TrimSpace(s.Size) != "" {
		out = append(out, "size")
	}
	if s.Rating > 0 {
		out = append(out, "rating")
	}
	if s.HasURL != "" {
		out = append(out, "url")
	}
	if s.HasNote != "" {
		out = append(out, "note")
	}
	if s.Untagged {
		out = append(out, "untagged")
	}
	if s.Uncategorized {
		out = append(out, "uncategorized")
	}
	return out
}

// Complete reports whether every selected filter can be sent to /items.
func (s Spec) Complete() bool { return len(s.Unsupported()) == 0 }

// Dims returns the dimensions the search page exposes, for chips.
func Dims() (types, orientations, sizes []string) {
	return []string{"image", "video", "audio"},
		[]string{"landscape", "portrait", "square"},
		[]string{"small", "medium", "large"}
}
