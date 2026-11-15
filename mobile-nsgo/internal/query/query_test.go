package query

import (
	"reflect"
	"testing"
)

func TestSpecStringMatchesLegacyGrammar(t *testing.T) {
	spec := Spec{
		Text:          `tag:foo "bar"`,
		Type:          "image",
		Orientation:   "landscape",
		Tag:           "family photos",
		Folder:        "trips:2024",
		Color:         "#AABBCC",
		Size:          "large",
		Rating:        4,
		HasURL:        "has",
		HasNote:       "empty",
		Untagged:      true,
		Uncategorized: true,
	}
	got := spec.String()
	want := `"tag:foo \"bar\"" type:image orientation:landscape is:untagged is:uncategorized tag:"family photos" folder:"trips:2024" color:#AABBCC size:large rating:4 has:url no:note`
	if got != want {
		t.Fatalf("String() = %q\nwant      %q", got, want)
	}
}

func TestQuote(t *testing.T) {
	cases := map[string]string{
		"image":         "image",
		"family photos": `"family photos"`,
		`a"b`:           `"a\"b"`,
		`a\b`:           `a\\b`,
		"tag:foo":       `"tag:foo"`,
	}
	for in, want := range cases {
		if got := Quote(in); got != want {
			t.Errorf("Quote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUnsupportedKeepsFallbackHonest(t *testing.T) {
	full := Spec{Text: "cat", Type: "image", Tag: "x", Folder: "y", Color: "#112233", Orientation: "square"}
	if !full.Complete() || len(full.Unsupported()) != 0 {
		t.Fatalf("basic filters should be complete, got %v", full.Unsupported())
	}
	advanced := Spec{Size: "small", Rating: 3, HasURL: "empty", HasNote: "has", Untagged: true, Uncategorized: true}
	want := []string{"size", "rating", "url", "note", "untagged", "uncategorized"}
	if got := advanced.Unsupported(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Unsupported() = %v, want %v", got, want)
	}
	if advanced.Complete() {
		t.Fatal("advanced-only filters must not report complete")
	}
}

func TestLegacyParamsDropsUnsupportedFields(t *testing.T) {
	spec := Spec{Text: " cat ", Type: "video", Size: "large", Rating: 5, Untagged: true}
	params := spec.LegacyParams()
	if got := params.Get("q"); got != "cat" {
		t.Errorf("q = %q", got)
	}
	if got := params.Get("type"); got != "video" {
		t.Errorf("type = %q", got)
	}
	for _, key := range []string{"size", "rating", "is"} {
		if params.Has(key) {
			t.Errorf("legacy params leaked %q", key)
		}
	}
}
