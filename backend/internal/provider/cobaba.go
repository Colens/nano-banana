package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image-gen-service/internal/diagnostic"
	"image-gen-service/internal/model"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// CobabaProvider 对接 Cobaba / 同类中转站的统一出图与 Nano Banana 异步接口：
//   - POST {base}/api/generate
//   - POST {base}/draw/nano-banana
//   - POST {base}/draw/result
type CobabaProvider struct {
	config     *model.ProviderConfig
	httpClient *http.Client
	apiBase    string
	userAgent  string
}

func NewCobabaProvider(config *model.ProviderConfig) (*CobabaProvider, error) {
	if config == nil {
		return nil, fmt.Errorf("config 不能为空")
	}

	timeout := time.Duration(config.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 500 * time.Second
	}

	return &CobabaProvider{
		config:     config,
		httpClient: newOpenAIHTTPClient(timeout),
		apiBase:    NormalizeCobabaBaseURL(config.APIBase),
		userAgent:  "image-gen-service/1.0",
	}, nil
}

func (p *CobabaProvider) Name() string {
	return "cobaba"
}

func (p *CobabaProvider) ValidateParams(params map[string]interface{}) error {
	prompt, _ := params["prompt"].(string)
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("prompt 不能为空")
	}
	return nil
}

func (p *CobabaProvider) Generate(ctx context.Context, params map[string]interface{}) (*ProviderResult, error) {
	modelID := ResolveModelID(ModelResolveOptions{
		ProviderName: p.Name(),
		Purpose:      PurposeImage,
		Params:       params,
		Config:       p.config,
	}).ID
	if modelID == "" {
		return nil, fmt.Errorf("缺少 model_id 参数")
	}

	prompt, _ := params["prompt"].(string)
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil, fmt.Errorf("缺少 prompt 参数")
	}

	aspectRatio := firstStringParam(params, "aspect_ratio", "aspectRatio", "aspect")
	if aspectRatio == "" {
		aspectRatio = "1:1"
	}
	imageSize := firstStringParam(params, "resolution_level", "imageSize", "image_size", "resolution")
	if imageSize == "" {
		imageSize = "1K"
	}
	imageSize = strings.ToUpper(strings.TrimSpace(imageSize))

	refImages, err := cobabaReferenceImages(params["reference_images"])
	if err != nil {
		return nil, err
	}

	diagnostic.Logf(params, "request_prepare",
		"provider=%s model=%s aspect_ratio=%q image_size=%q ref_image_count=%d prompt_hash=%s prompt_preview=%q",
		p.Name(),
		modelID,
		aspectRatio,
		imageSize,
		len(refImages),
		diagnostic.PromptHash(prompt),
		diagnostic.Preview(prompt, 160),
	)

	images, meta, err := p.generateViaAPIGenerate(ctx, modelID, prompt, aspectRatio, imageSize, refImages, params)
	if err != nil {
		log.Printf("[Cobaba] /api/generate 失败，回退 /draw/nano-banana: %v", err)
		images, meta, err = p.generateViaNanoBanana(ctx, modelID, prompt, aspectRatio, refImages, params)
		if err != nil {
			return nil, err
		}
	}

	if meta == nil {
		meta = map[string]interface{}{}
	}
	meta["provider"] = p.Name()
	meta["model"] = modelID
	meta["type"] = "image"

	return &ProviderResult{Images: images, Metadata: meta}, nil
}

func (p *CobabaProvider) generateViaAPIGenerate(
	ctx context.Context,
	modelID, prompt, aspectRatio, imageSize string,
	refImages []string,
	params map[string]interface{},
) ([][]byte, map[string]interface{}, error) {
	body := map[string]interface{}{
		"model":       modelID,
		"prompt":      prompt,
		"images":      refImages,
		"aspectRatio": aspectRatio,
		"imageSize":   imageSize,
		"replyType":   "async",
	}

	respBytes, headers, err := p.doJSONPost(ctx, p.apiBase+"/api/generate", body, params)
	if err != nil {
		return nil, nil, err
	}

	images, err := p.extractImagesFlexible(ctx, respBytes)
	if err == nil && len(images) > 0 {
		return images, map[string]interface{}{
			"request_id":     extractRequestIDFromHeaders(headers),
			"endpoint":       "api/generate",
			"oneapi_request": strings.TrimSpace(headers.Get("X-Oneapi-Request-Id")),
		}, nil
	}

	taskID := extractCobabaTaskID(respBytes)
	if taskID == "" {
		if err != nil {
			return nil, nil, fmt.Errorf("api/generate 未返回图片且无任务 ID: %w", err)
		}
		return nil, nil, fmt.Errorf("api/generate 未返回图片且无任务 ID: %s", diagnostic.Preview(string(respBytes), 400))
	}

	images, err = p.pollDrawResult(ctx, taskID, params)
	if err != nil {
		return nil, nil, err
	}
	return images, map[string]interface{}{
		"request_id": extractRequestIDFromHeaders(headers),
		"endpoint":   "api/generate",
		"task_id":    taskID,
	}, nil
}

func (p *CobabaProvider) generateViaNanoBanana(
	ctx context.Context,
	modelID, prompt, aspectRatio string,
	refImages []string,
	params map[string]interface{},
) ([][]byte, map[string]interface{}, error) {
	body := map[string]interface{}{
		"model":       modelID,
		"prompt":      prompt,
		"aspectRatio": aspectRatio,
		"images":      refImages,
		"webHook":     "-1",
	}

	respBytes, headers, err := p.doJSONPost(ctx, p.apiBase+"/draw/nano-banana", body, params)
	if err != nil {
		return nil, nil, err
	}

	images, err := p.extractImagesFlexible(ctx, respBytes)
	if err == nil && len(images) > 0 {
		return images, map[string]interface{}{
			"request_id": extractRequestIDFromHeaders(headers),
			"endpoint":   "draw/nano-banana",
		}, nil
	}

	taskID := extractCobabaTaskID(respBytes)
	if taskID == "" {
		if err != nil {
			return nil, nil, fmt.Errorf("draw/nano-banana 未返回图片且无任务 ID: %w", err)
		}
		return nil, nil, fmt.Errorf("draw/nano-banana 未返回图片且无任务 ID: %s", diagnostic.Preview(string(respBytes), 400))
	}

	images, err = p.pollDrawResult(ctx, taskID, params)
	if err != nil {
		return nil, nil, err
	}
	return images, map[string]interface{}{
		"request_id": extractRequestIDFromHeaders(headers),
		"endpoint":   "draw/nano-banana",
		"task_id":    taskID,
	}, nil
}

func (p *CobabaProvider) pollDrawResult(ctx context.Context, taskID string, params map[string]interface{}) ([][]byte, error) {
	deadline := time.Now().Add(8 * time.Minute)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}

	var lastErr error
	for attempt := 1; time.Now().Before(deadline); attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		respBytes, _, err := p.doJSONPost(ctx, p.apiBase+"/draw/result", map[string]interface{}{"id": taskID}, params)
		if err != nil {
			lastErr = err
			log.Printf("[Cobaba] poll result 第%d次失败 task=%s err=%v", attempt, taskID, err)
			if sleepErr := sleepWithContext(ctx, 2*time.Second); sleepErr != nil {
				return nil, sleepErr
			}
			continue
		}

		status := strings.ToUpper(extractCobabaStatus(respBytes))
		diagnostic.Logf(params, "poll_status", "task_id=%s attempt=%d status=%q body_preview=%q",
			taskID, attempt, status, diagnostic.Preview(string(respBytes), 240))

		switch status {
		case "FAILED", "FAILURE", "ERROR":
			reason := extractCobabaFailReason(respBytes)
			if reason == "" {
				reason = diagnostic.Preview(string(respBytes), 400)
			}
			return nil, fmt.Errorf("任务失败: %s", reason)
		case "SUCCESS", "SUCCEEDED", "COMPLETED", "DONE", "FINISH", "FINISHED":
			images, extractErr := p.extractImagesFlexible(ctx, respBytes)
			if extractErr != nil {
				return nil, extractErr
			}
			if len(images) == 0 {
				return nil, fmt.Errorf("任务成功但未解析到图片数据")
			}
			return images, nil
		}

		// 无明确状态时：若已能解出图片则视为成功
		if images, extractErr := p.extractImagesFlexible(ctx, respBytes); extractErr == nil && len(images) > 0 {
			return images, nil
		}

		if sleepErr := sleepWithContext(ctx, 2*time.Second); sleepErr != nil {
			return nil, sleepErr
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("轮询任务超时: %w", lastErr)
	}
	return nil, fmt.Errorf("轮询任务超时: task_id=%s", taskID)
}

func (p *CobabaProvider) doJSONPost(ctx context.Context, url string, body map[string]interface{}, params map[string]interface{}) ([]byte, http.Header, error) {
	payloadBytes, err := json.Marshal(body)
	if err != nil {
		return nil, nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	diagnostic.Logf(params, "request_payload",
		"url=%s body=%q",
		diagnostic.RedactSensitive(url),
		diagnostic.RedactSensitive(string(payloadBytes)),
	)

	maxRetries := providerMaxRetries(p.config)
	var elapsed time.Duration
	resp, _, err := doRequestWithRetry(ctx, params, p.Name(), maxRetries, func(attempt int) (*http.Response, error) {
		req, buildErr := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payloadBytes))
		if buildErr != nil {
			return nil, fmt.Errorf("构建请求失败: %w", buildErr)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(p.config.APIKey))
		req.Header.Set("Connection", "close")
		if strings.TrimSpace(p.userAgent) != "" {
			req.Header.Set("User-Agent", p.userAgent)
		}
		startedAt := time.Now()
		resp, doErr := p.httpClient.Do(req)
		elapsed = time.Since(startedAt)
		return resp, doErr
	})
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.Header.Clone(), fmt.Errorf("读取响应失败: %w", err)
	}

	requestID := extractRequestIDFromHeaders(resp.Header)
	diagnostic.Logf(params, "response_body",
		"status=%s elapsed=%s request_id=%s body_length=%d body_preview=%q",
		resp.Status,
		elapsed,
		requestID,
		len(respBody),
		diagnostic.Preview(diagnostic.RedactSensitive(string(respBody)), 1200),
	)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.Header.Clone(), fmt.Errorf("HTTP %d request_id=%s %s", resp.StatusCode, requestID, openAIErrorBodyPreview(respBody, 1200))
	}
	if len(respBody) == 0 {
		return nil, resp.Header.Clone(), fmt.Errorf("接口未返回内容")
	}
	return respBody, resp.Header.Clone(), nil
}

func (p *CobabaProvider) extractImagesFlexible(ctx context.Context, respBytes []byte) ([][]byte, error) {
	var images [][]byte

	// 1) OpenAI Images 风格 data[]
	var raw map[string]interface{}
	if err := json.Unmarshal(respBytes, &raw); err == nil {
		if data, ok := raw["data"].([]interface{}); ok && len(data) > 0 {
			if imgs, err := extractImagesFromOpenAIData(ctx, p.httpClient, data); err == nil {
				images = append(images, imgs...)
			}
		}
		// data 也可能是对象
		if dataObj, ok := raw["data"].(map[string]interface{}); ok {
			images = append(images, collectImagesFromAny(ctx, p.httpClient, dataObj)...)
		}
		images = append(images, collectImagesFromAny(ctx, p.httpClient, raw)...)
	}

	// 2) 正文中的 data URL / markdown URL
	text := string(respBytes)
	images = append(images, extractImagesFromText(text)...)
	for _, url := range extractMarkdownImageURLs(text) {
		if img, err := fetchImageWithClient(ctx, p.httpClient, url); err == nil {
			images = append(images, img)
		} else {
			log.Printf("[Cobaba] 下载 markdown 图片失败 url=%s err=%v", url, err)
		}
	}
	for _, url := range extractBareImageURLs(text) {
		if img, err := fetchImageWithClient(ctx, p.httpClient, url); err == nil {
			images = append(images, img)
		}
	}

	images = dedupeImageBytes(images)
	if len(images) == 0 {
		return nil, fmt.Errorf("未在响应中找到图片数据: %s", diagnostic.Preview(text, 300))
	}
	return images, nil
}

func collectImagesFromAny(ctx context.Context, client *http.Client, node interface{}) [][]byte {
	var images [][]byte
	switch v := node.(type) {
	case map[string]interface{}:
		for key, val := range v {
			lk := strings.ToLower(key)
			switch typed := val.(type) {
			case string:
				s := strings.TrimSpace(typed)
				if s == "" {
					continue
				}
				if strings.HasPrefix(s, "data:image/") {
					if img, err := decodeDataURL(s); err == nil {
						images = append(images, img)
					}
					continue
				}
				if looksLikeImageURL(s) || strings.Contains(lk, "image") || strings.Contains(lk, "url") || lk == "content" || lk == "result" {
					for _, url := range extractAllImageURLs(s) {
						if img, err := fetchImageWithClient(ctx, client, url); err == nil {
							images = append(images, img)
						}
					}
					if looksLikeImageURL(s) {
						if img, err := fetchImageWithClient(ctx, client, s); err == nil {
							images = append(images, img)
						}
					}
					if b64 := maybeRawBase64Image(s); len(b64) > 0 {
						images = append(images, b64)
					}
				}
			case []interface{}:
				if strings.Contains(lk, "image") || lk == "urls" || lk == "data" {
					for _, item := range typed {
						images = append(images, collectImagesFromAny(ctx, client, item)...)
					}
				} else {
					for _, item := range typed {
						images = append(images, collectImagesFromAny(ctx, client, item)...)
					}
				}
			case map[string]interface{}:
				images = append(images, collectImagesFromAny(ctx, client, typed)...)
			}
		}
	case string:
		s := strings.TrimSpace(v)
		for _, url := range extractAllImageURLs(s) {
			if img, err := fetchImageWithClient(ctx, client, url); err == nil {
				images = append(images, img)
			}
		}
		if b64 := maybeRawBase64Image(s); len(b64) > 0 {
			images = append(images, b64)
		}
	case []interface{}:
		for _, item := range v {
			images = append(images, collectImagesFromAny(ctx, client, item)...)
		}
	}
	return images
}

func extractImagesFromOpenAIData(ctx context.Context, client *http.Client, data []interface{}) ([][]byte, error) {
	var images [][]byte
	for _, item := range data {
		obj, ok := item.(map[string]interface{})
		if !ok {
			// data 元素可能直接是 URL 字符串
			if s, ok := item.(string); ok {
				for _, url := range extractAllImageURLs(s) {
					if img, err := fetchImageWithClient(ctx, client, url); err == nil {
						images = append(images, img)
					}
				}
			}
			continue
		}
		if b64, ok := obj["b64_json"].(string); ok && b64 != "" {
			imgBytes, err := base64.StdEncoding.DecodeString(b64)
			if err == nil {
				images = append(images, imgBytes)
			}
			continue
		}
		if url, ok := obj["url"].(string); ok && url != "" {
			if img, err := fetchImageWithClient(ctx, client, url); err == nil {
				images = append(images, img)
			}
		}
	}
	return images, nil
}

func cobabaReferenceImages(raw interface{}) ([]string, error) {
	refImgs, ok := raw.([]interface{})
	if !ok || len(refImgs) == 0 {
		return []string{}, nil
	}

	out := make([]string, 0, len(refImgs))
	for idx, ref := range refImgs {
		var imgBytes []byte
		switch v := ref.(type) {
		case string:
			base64Data := v
			if strings.Contains(base64Data, ",") {
				base64Data = strings.SplitN(base64Data, ",", 2)[1]
			}
			decoded, err := base64.StdEncoding.DecodeString(base64Data)
			if err != nil {
				return nil, fmt.Errorf("解码第 %d 张参考图失败: %w", idx+1, err)
			}
			imgBytes = decoded
		case []byte:
			imgBytes = v
		default:
			continue
		}
		if len(imgBytes) == 0 {
			continue
		}
		mimeType := http.DetectContentType(imgBytes)
		if !strings.HasPrefix(mimeType, "image/") {
			mimeType = "image/png"
		}
		out = append(out, fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(imgBytes)))
	}
	return out, nil
}

func NormalizeCobabaBaseURL(apiBase string) string {
	base := strings.TrimSpace(apiBase)
	if base == "" {
		return "https://cobabaai.com/v1"
	}
	base = strings.TrimRight(base, "/")
	for _, suffix := range []string{
		"/api/generate",
		"/draw/nano-banana",
		"/draw/result",
		"/draw/completions",
		"/chat/completions",
		"/images/generations",
	} {
		if strings.HasSuffix(strings.ToLower(base), suffix) {
			base = base[:len(base)-len(suffix)]
			base = strings.TrimRight(base, "/")
		}
	}
	if strings.HasSuffix(base, "/v1") {
		return base
	}
	if strings.Contains(base, "/v1/") {
		return strings.Split(base, "/v1/")[0] + "/v1"
	}
	return base + "/v1"
}

func extractCobabaTaskID(resp []byte) string {
	var raw map[string]interface{}
	if err := json.Unmarshal(resp, &raw); err != nil {
		// 纯文本任务 ID
		text := strings.TrimSpace(string(resp))
		if strings.HasPrefix(text, "task_") || regexp.MustCompile(`(?i)^[0-9a-f-]{16,}$`).MatchString(text) {
			return text
		}
		return ""
	}

	candidates := []string{
		asString(raw["id"]),
		asString(raw["task_id"]),
		asString(raw["taskId"]),
		asString(raw["result"]),
	}
	if data, ok := raw["data"].(map[string]interface{}); ok {
		candidates = append(candidates,
			asString(data["id"]),
			asString(data["task_id"]),
			asString(data["taskId"]),
			asString(data["result"]),
		)
	}
	if data, ok := raw["data"].(string); ok {
		candidates = append(candidates, data)
	}
	for _, c := range candidates {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if strings.HasPrefix(c, "task_") || regexp.MustCompile(`(?i)^[0-9a-f-]{8,}$`).MatchString(c) {
			return c
		}
		// 有些接口直接返回 UUID
		if len(c) >= 8 && !strings.Contains(c, " ") && !strings.HasPrefix(c, "http") {
			return c
		}
	}
	return ""
}

func extractCobabaStatus(resp []byte) string {
	var raw map[string]interface{}
	if err := json.Unmarshal(resp, &raw); err != nil {
		return ""
	}
	if s := asString(raw["status"]); s != "" {
		return s
	}
	if data, ok := raw["data"].(map[string]interface{}); ok {
		if s := asString(data["status"]); s != "" {
			return s
		}
		if s := asString(data["state"]); s != "" {
			return s
		}
	}
	return ""
}

func extractCobabaFailReason(resp []byte) string {
	var raw map[string]interface{}
	if err := json.Unmarshal(resp, &raw); err != nil {
		return ""
	}
	for _, key := range []string{"failReason", "fail_reason", "error", "message"} {
		if s := asString(raw[key]); s != "" {
			return s
		}
	}
	if data, ok := raw["data"].(map[string]interface{}); ok {
		for _, key := range []string{"failReason", "fail_reason", "error", "message"} {
			if s := asString(data[key]); s != "" {
				return s
			}
		}
	}
	return ""
}

func asString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%v", t)
	default:
		return ""
	}
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
