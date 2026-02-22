package cli

import (
	"strings"
	"testing"
)

func TestNormalizePhotoEditFormat(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "png", want: "png"},
		{in: "jpg", want: "jpg"},
		{in: "jpeg", want: "jpeg"},
		{in: "webp", want: "webp"},
		{in: "gif", wantErr: true},
	}
	for _, tc := range tests {
		got, err := normalizePhotoEditFormat(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("expected error for %q", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("normalizePhotoEditFormat(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestResolvePhotoEditOutputPathDefault(t *testing.T) {
	got, err := resolvePhotoEditOutputPath("/repo", "", "images/hero.png", "webp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantSuffix := "/repo/images/hero.edited.webp"
	if !strings.HasSuffix(got, wantSuffix) {
		t.Fatalf("unexpected output path: got %q want suffix %q", got, wantSuffix)
	}
}

func TestSummarizePhotoEditErrorJSON(t *testing.T) {
	body := []byte(`{"errorCode":"INVALID_IMAGE","errorDescription":"unsupported format"}`)
	got := summarizePhotoEditError(body)
	if !strings.Contains(got, "INVALID_IMAGE") || !strings.Contains(got, "unsupported format") {
		t.Fatalf("unexpected summary: %q", got)
	}
}
