package provider

import "testing"

func TestResolveCobabaAspectRatioGPTImage(t *testing.T) {
	got := resolveCobabaAspectRatio("gpt-image-2", "16:9", "4K")
	if got != "1672x941" {
		t.Fatalf("gpt-image-2 16:9 = %q, want 1672x941", got)
	}

	got = resolveCobabaAspectRatio("gpt-image-2.5", "1:1", "2K")
	if got != "1024x1024" {
		t.Fatalf("gpt-image-2.5 1:1 = %q, want 1024x1024", got)
	}

	got = resolveCobabaAspectRatio("gpt-image-2", "auto", "2K")
	if got != "auto" {
		t.Fatalf("gpt-image-2 auto = %q, want auto", got)
	}
}

func TestResolveCobabaAspectRatioVIP(t *testing.T) {
	got := resolveCobabaAspectRatio("gpt-image-2-vip", "16:9", "2K")
	if got != "2048x1152" {
		t.Fatalf("vip 16:9 2K = %q, want 2048x1152", got)
	}

	got = resolveCobabaAspectRatio("gpt-image-2.5-flare", "1:1", "4K")
	if got != "2880x2880" {
		t.Fatalf("flare 1:1 4K = %q, want 2880x2880", got)
	}

	got = resolveCobabaAspectRatio("gpt-image-2.5-sunburst", "1:3", "4K")
	if got != "1280x3840" {
		t.Fatalf("sunburst 1:3 4K fallback = %q, want 1280x3840", got)
	}

	got = resolveCobabaAspectRatio("gpt-image-2-vip", "1024x1024", "4K")
	if got != "1024x1024" {
		t.Fatalf("explicit pixel size = %q, want 1024x1024", got)
	}
}

func TestResolveCobabaQuality(t *testing.T) {
	if got := resolveCobabaQuality("gpt-image-2", "high"); got != "auto" {
		t.Fatalf("gpt-image-2 quality = %q, want auto", got)
	}
	if got := resolveCobabaQuality("gpt-image-2-vip", "auto"); got != "medium" {
		t.Fatalf("vip quality = %q, want medium", got)
	}
	if got := resolveCobabaQuality("gpt-image-2.5-flare", "high"); got != "high" {
		t.Fatalf("flare quality = %q, want high", got)
	}
	if got := resolveCobabaQuality("gpt-image-2.5-sunburst", "max"); got != "max" {
		t.Fatalf("sunburst quality = %q, want max", got)
	}
	if got := resolveCobabaQuality("nano-banana-2", "high"); got != "" {
		t.Fatalf("nano-banana quality = %q, want empty", got)
	}
}

func TestCobabaTerminalError(t *testing.T) {
	if err := cobabaTerminalError([]byte(`{"status":"running"}`)); err != nil {
		t.Fatalf("running should not be terminal: %v", err)
	}
	if err := cobabaTerminalError([]byte(`{"status":"failed","error":"generate failed"}`)); err == nil {
		t.Fatal("failed should be terminal")
	}
	if err := cobabaTerminalError([]byte(`{"status":"violation","error":"nsfw"}`)); err == nil {
		t.Fatal("violation should be terminal")
	}
}
