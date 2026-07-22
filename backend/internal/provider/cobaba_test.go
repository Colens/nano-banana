package provider

import (
	"testing"
)

func TestNormalizeCobabaBaseURL(t *testing.T) {
	cases := map[string]string{
		"":                                  "https://cobabaai.com/v1",
		"https://cobabaai.com":              "https://cobabaai.com/v1",
		"https://cobabaai.com/v1":           "https://cobabaai.com/v1",
		"https://cobabaai.com/v1/":          "https://cobabaai.com/v1",
		"https://cobabaai.com/v1/api/generate": "https://cobabaai.com/v1",
		"https://cobabaai.com/v1/draw/nano-banana": "https://cobabaai.com/v1",
	}
	for input, want := range cases {
		if got := NormalizeCobabaBaseURL(input); got != want {
			t.Fatalf("NormalizeCobabaBaseURL(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestExtractMarkdownImageURLs(t *testing.T) {
	text := `未在响应中找到图片数据: ![image](https://file6.aitohumanize.com/file/6b90c47b652d4e2aac9ec91f90a8e3a4.png)`
	urls := extractMarkdownImageURLs(text)
	if len(urls) != 1 {
		t.Fatalf("expected 1 url, got %v", urls)
	}
	if urls[0] != "https://file6.aitohumanize.com/file/6b90c47b652d4e2aac9ec91f90a8e3a4.png" {
		t.Fatalf("unexpected url: %s", urls[0])
	}
}

func TestExtractAllImageURLsIncludesCDNFile(t *testing.T) {
	text := `ok https://file6.aitohumanize.com/file/abc123`
	urls := extractAllImageURLs(text)
	if len(urls) == 0 {
		t.Fatal("expected CDN file url")
	}
}
