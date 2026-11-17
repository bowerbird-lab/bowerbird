package app

import (
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/transfer"
)

func TestDropURLsSkipsHTMLFallbackWhenPluginClaimsURL(t *testing.T) {
	canResolve := func(raw string) bool {
		return strings.Contains(raw, "pinterest.com/pin/")
	}

	data := transfer.New(transfer.NewItem(
		transfer.Bytes(transfer.URIList, []byte("https://www.pinterest.com/pin/123456/\r\n")),
		transfer.Bytes(transfer.Text, []byte("https://www.pinterest.com/pin/123456/")),
		transfer.Bytes(transfer.HTML, []byte(`<a href="https://www.pinterest.com/pin/123456/"><img src="https://i.pinimg.com/236x/ab/cd/ef.jpg" srcset="https://i.pinimg.com/474x/ab/cd/ef.jpg 2x"></a>`)),
	))

	urls, referer := urlsFromDropDataWithResolver(data, canResolve)
	want := []string{"https://www.pinterest.com/pin/123456/"}
	if fmt.Sprint(urls) != fmt.Sprint(want) {
		t.Fatalf("urls = %v, want %v", urls, want)
	}
	if referer != "https://www.pinterest.com/pin/123456/" {
		t.Fatalf("referer = %q, want pin URL", referer)
	}
}

func TestDropURLsPrefersPluginPageWhenOnlyHTMLPresent(t *testing.T) {
	canResolve := func(raw string) bool {
		return strings.Contains(raw, "pinterest.com/pin/")
	}

	data := transfer.New(transfer.NewItem(
		transfer.Bytes(transfer.HTML, []byte(`<a href="https://www.pinterest.com/pin/123456/"><img src="https://i.pinimg.com/236x/ab/cd/ef.jpg"></a>`)),
	))

	urls, referer := urlsFromDropDataWithResolver(data, canResolve)
	want := []string{"https://www.pinterest.com/pin/123456/"}
	if fmt.Sprint(urls) != fmt.Sprint(want) {
		t.Fatalf("urls = %v, want %v", urls, want)
	}
	if referer != "https://www.pinterest.com/pin/123456/" {
		t.Fatalf("referer = %q, want pin URL", referer)
	}
}

func TestDropURLsFallsBackToHTMLForUnmatchedWebPage(t *testing.T) {
	canResolve := func(string) bool { return false }

	data := transfer.New(transfer.NewItem(
		transfer.Bytes(transfer.URIList, []byte("https://example.com/post/42\r\n")),
		transfer.Bytes(transfer.HTML, []byte(`<a href="https://example.com/post/42"><img src="https://example.com/images/photo.jpg"></a>`)),
	))

	urls, referer := urlsFromDropDataWithResolver(data, canResolve)
	want := []string{"https://example.com/images/photo.jpg"}
	if fmt.Sprint(urls) != fmt.Sprint(want) {
		t.Fatalf("urls = %v, want %v", urls, want)
	}
	if referer != "https://example.com/post/42" {
		t.Fatalf("referer = %q, want https://example.com/post/42", referer)
	}
}

func TestDropURLsKeepsDirectMediaWithoutHTMLThumbnails(t *testing.T) {
	canResolve := func(string) bool { return false }

	data := transfer.New(transfer.NewItem(
		transfer.Bytes(transfer.URIList, []byte("https://example.com/full.jpg\r\n")),
		transfer.Bytes(transfer.HTML, []byte(`<a href="https://example.com/post/42"><img src="https://example.com/thumb.jpg"></a>`)),
	))

	urls, referer := urlsFromDropDataWithResolver(data, canResolve)
	want := []string{"https://example.com/full.jpg"}
	if fmt.Sprint(urls) != fmt.Sprint(want) {
		t.Fatalf("urls = %v, want %v", urls, want)
	}
	if referer != "https://example.com/post/42" {
		t.Fatalf("referer = %q, want https://example.com/post/42", referer)
	}
}
