package provider

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var (
	markdownImageURLRe = regexp.MustCompile(`!\[[^\]]*\]\((https?://[^)\s]+)\)`)
	bareImageURLRe     = regexp.MustCompile(`https?://[^\s"'<>]+\.(?:png|jpe?g|webp|gif)(?:\?[^\s"'<>]*)?`)
	httpURLRe          = regexp.MustCompile(`https?://[^\s"'<>)]+`)
)

func extractMarkdownImageURLs(text string) []string {
	matches := markdownImageURLRe.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	seen := map[string]struct{}{}
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		url := strings.TrimSpace(m[1])
		if url == "" {
			continue
		}
		if _, ok := seen[url]; ok {
			continue
		}
		seen[url] = struct{}{}
		out = append(out, url)
	}
	return out
}

func extractBareImageURLs(text string) []string {
	matches := bareImageURLRe.FindAllString(text, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	seen := map[string]struct{}{}
	for _, url := range matches {
		url = strings.TrimRight(url, ".,);]")
		if _, ok := seen[url]; ok {
			continue
		}
		seen[url] = struct{}{}
		out = append(out, url)
	}
	return out
}

func extractAllImageURLs(text string) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(url string) {
		url = strings.TrimSpace(url)
		url = strings.TrimRight(url, ".,);]")
		if url == "" {
			return
		}
		if _, ok := seen[url]; ok {
			return
		}
		seen[url] = struct{}{}
		out = append(out, url)
	}
	for _, url := range extractMarkdownImageURLs(text) {
		add(url)
	}
	for _, url := range extractBareImageURLs(text) {
		add(url)
	}
	// 某些中转站返回无扩展名的 CDN 链接（如 file*.xxx.com/file/<id>）
	for _, url := range httpURLRe.FindAllString(text, -1) {
		lower := strings.ToLower(url)
		if strings.Contains(lower, "/file/") ||
			strings.Contains(lower, "aitohumanize") ||
			strings.Contains(lower, "cobaba") ||
			strings.Contains(lower, "oss") ||
			strings.Contains(lower, "cdn") {
			add(url)
		}
	}
	return out
}

func looksLikeImageURL(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return false
	}
	if strings.Contains(lower, ".png") ||
		strings.Contains(lower, ".jpg") ||
		strings.Contains(lower, ".jpeg") ||
		strings.Contains(lower, ".webp") ||
		strings.Contains(lower, ".gif") {
		return true
	}
	return strings.Contains(lower, "/file/") ||
		strings.Contains(lower, "image") ||
		strings.Contains(lower, "cdn")
}

func maybeRawBase64Image(s string) []byte {
	s = strings.TrimSpace(s)
	if len(s) < 256 {
		return nil
	}
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "data:") {
		return nil
	}
	if strings.ContainsAny(s, " \n\t") {
		return nil
	}
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(s)
		if err != nil {
			return nil
		}
	}
	if len(decoded) < 32 {
		return nil
	}
	mime := http.DetectContentType(decoded)
	if !strings.HasPrefix(mime, "image/") {
		return nil
	}
	return decoded
}

func fetchImageWithClient(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	if strings.HasPrefix(url, "data:image/") {
		return decodeDataURL(url)
	}
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}

	const maxRetries = 3
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			log.Printf("[fetchImage] 第%d次尝试失败 url=%s err=%v", attempt, url, err)
			if attempt < maxRetries {
				time.Sleep(time.Second)
			}
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			if attempt < maxRetries {
				time.Sleep(time.Second)
			}
			continue
		}

		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("下载图片失败: %s", resp.Status)
			log.Printf("[fetchImage] 第%d次尝试失败 url=%s status=%s", attempt, url, resp.Status)
			if attempt < maxRetries {
				time.Sleep(time.Second)
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("下载图片失败: %s", resp.Status)
		}
		return body, nil
	}
	if lastErr == nil {
		return nil, fmt.Errorf("下载图片失败")
	}
	return nil, fmt.Errorf("下载图片失败（重试%d次）: %w", maxRetries, lastErr)
}

func dedupeImageBytes(images [][]byte) [][]byte {
	if len(images) <= 1 {
		return images
	}
	seen := map[string]struct{}{}
	out := make([][]byte, 0, len(images))
	for _, img := range images {
		if len(img) == 0 {
			continue
		}
		n := 16
		if len(img) < n {
			n = len(img)
		}
		key := fmt.Sprintf("%d:%x", len(img), img[:n])
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, img)
	}
	return out
}
