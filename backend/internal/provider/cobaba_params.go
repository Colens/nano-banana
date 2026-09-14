package provider

import (
	"strconv"
	"strings"
)

const (
	cobabaResolution1K = "1K"
	cobabaResolution2K = "2K"
	cobabaResolution4K = "4K"
)

// gpt-image-2 / gpt-image-2.5 只支持比例或 1K 像素值。
var cobabaGPTImage1KSizes = map[string]string{
	"auto": "auto",
	"1:1":  "1024x1024",
	"16:9": "1672x941",
	"9:16": "941x1672",
	"4:3":  "1443x1090",
	"3:4":  "1090x1443",
	"3:2":  "1536x1024",
	"2:3":  "1024x1536",
	"5:4":  "1408x1120",
	"4:5":  "1120x1408",
	"21:9": "1920x832",
	"9:21": "832x1920",
	"1:2":  "896x1792",
	"2:1":  "1792x896",
}

// gpt-image-2-vip / gpt-image-2.5-flare / gpt-image-2.5-sunburst 必须传像素值，不支持比例。
// 数组依次对应 1K、2K、4K；空字符串表示该档位文档未给出。
var cobabaGPTImageVIPSizes = map[string][3]string{
	"auto": {},
	"1:1":  {"1024x1024", "2048x2048", "2880x2880"},
	"16:9": {"1280x720", "2048x1152", "3840x2160"},
	"9:16": {"720x1280", "1152x2048", "2160x3840"},
	"4:3":  {"1152x864", "2304x1728", "3264x2448"},
	"3:4":  {"864x1152", "1728x2304", "2448x3264"},
	"3:2":  {"1536x1024", "2048x1360", "3504x2336"},
	"2:3":  {"1024x1536", "1360x2048", "2336x3504"},
	"5:4":  {"1120x896", "2240x1792", "3200x2560"},
	"4:5":  {"896x1120", "1792x2240", "2560x3200"},
	"21:9": {"1456x624", "2912x1248", "3840x1648"},
	"9:21": {"624x1456", "1248x2912", "1648x3840"},
	"1:3":  {"688x2048", "1280x3840", ""},
	"3:1":  {"2048x688", "3840x1280", ""},
	"2:1":  {"1536x768", "3072x1536", "3840x1920"},
	"1:2":  {"768x1536", "1536x3072", "1920x3840"},
}

func normalizeCobabaModelID(modelID string) string {
	return strings.ToLower(strings.TrimSpace(modelID))
}

func isCobabaGPTImageModel(modelID string) bool {
	return strings.HasPrefix(normalizeCobabaModelID(modelID), "gpt-image-2")
}

func isCobabaNanoBananaModel(modelID string) bool {
	return strings.Contains(normalizeCobabaModelID(modelID), "nano-banana")
}

func isCobabaVIPPixelModel(modelID string) bool {
	m := normalizeCobabaModelID(modelID)
	return strings.Contains(m, "vip") || strings.Contains(m, "flare") || strings.Contains(m, "sunburst")
}

func cobabaQualityOptions(modelID string) []string {
	m := normalizeCobabaModelID(modelID)
	switch {
	case strings.Contains(m, "sunburst"):
		return []string{"low", "medium", "high", "xhigh", "max"}
	case strings.Contains(m, "flare"):
		return []string{"low", "medium", "high"}
	case strings.Contains(m, "vip"):
		return []string{"medium"}
	case m == "gpt-image-2" || m == "gpt-image-2.5":
		return []string{"auto"}
	default:
		return nil
	}
}

func resolveCobabaQuality(modelID, quality string) string {
	allowed := cobabaQualityOptions(modelID)
	if len(allowed) == 0 {
		return ""
	}
	q := strings.ToLower(strings.TrimSpace(quality))
	for _, item := range allowed {
		if q == item {
			return item
		}
	}
	if strings.Contains(normalizeCobabaModelID(modelID), "flare") || strings.Contains(normalizeCobabaModelID(modelID), "sunburst") {
		return "medium"
	}
	return allowed[0]
}

func cobabaResolutionIndex(imageSize string) int {
	switch strings.ToUpper(strings.TrimSpace(imageSize)) {
	case cobabaResolution1K:
		return 0
	case cobabaResolution4K:
		return 2
	case cobabaResolution2K:
		fallthrough
	default:
		return 1
	}
}

func pickCobabaVIPSize(sizes [3]string, imageSize string) string {
	idx := cobabaResolutionIndex(imageSize)
	if idx >= 0 && idx < len(sizes) && strings.TrimSpace(sizes[idx]) != "" {
		return sizes[idx]
	}
	for i := len(sizes) - 1; i >= 0; i-- {
		if strings.TrimSpace(sizes[i]) != "" {
			return sizes[i]
		}
	}
	return "1024x1024"
}

func normalizeCobabaAspectKey(aspectRatio string) string {
	ar := strings.TrimSpace(aspectRatio)
	if ar == "" {
		return "1:1"
	}
	if isAutoAspectRatio(ar) {
		return "auto"
	}
	if looksLikePixelSize(ar) {
		return strings.ToLower(strings.ReplaceAll(ar, " ", ""))
	}
	w, h, ok := parseAspectRatio(ar)
	if !ok {
		return ar
	}
	return strconv.Itoa(w) + ":" + strconv.Itoa(h)
}

func looksLikePixelSize(value string) bool {
	value = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(value, " ", "")))
	return isValidOpenAIImageSize(value) && value != "auto"
}

func resolveCobabaAspectRatio(modelID, aspectRatio, imageSize string) string {
	ar := strings.TrimSpace(aspectRatio)
	if looksLikePixelSize(ar) {
		return strings.ToLower(strings.ReplaceAll(ar, " ", ""))
	}
	key := normalizeCobabaAspectKey(ar)
	if isCobabaVIPPixelModel(modelID) {
		if key == "auto" {
			return "auto"
		}
		if sizes, ok := cobabaGPTImageVIPSizes[key]; ok {
			return pickCobabaVIPSize(sizes, imageSize)
		}
		return computeDynamicOpenAIImageSize(key, imageSize)
	}
	if isCobabaGPTImageModel(modelID) {
		if size, ok := cobabaGPTImage1KSizes[key]; ok {
			return size
		}
		if key == "" {
			return "1024x1024"
		}
		return key
	}
	if ar == "" {
		return "1:1"
	}
	return ar
}
