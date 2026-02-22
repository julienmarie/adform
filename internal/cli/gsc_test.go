package cli

import "testing"

func TestParseGSCDimensions(t *testing.T) {
	req, out, err := parseGSCDimensions("query,page,country,platform")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(req) != 4 || len(out) != 4 {
		t.Fatalf("unexpected lengths req=%d out=%d", len(req), len(out))
	}
	if req[3] != "device" || out[3] != "platform" {
		t.Fatalf("expected platform alias to map to device/platform, got req=%q out=%q", req[3], out[3])
	}
}

func TestParseGSCDimensionsRejectsInvalid(t *testing.T) {
	if _, _, err := parseGSCDimensions("query,unknown"); err == nil {
		t.Fatal("expected error for invalid dimension")
	}
}

func TestNormalizeGSCType(t *testing.T) {
	got, err := normalizeGSCType("google_news")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "googleNews" {
		t.Fatalf("expected googleNews, got %q", got)
	}
}
