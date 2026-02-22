package cli

import (
	"strconv"
	"testing"
	"time"
)

func TestNormalizeLogBoundAcceptsDate(t *testing.T) {
	api, cmp, err := normalizeLogBound("2026-02-19")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if api != "2026-02-19" {
		t.Fatalf("expected date api value, got %q", api)
	}
	if cmp <= 0 {
		t.Fatalf("expected positive compare unix, got %d", cmp)
	}
}

func TestNormalizeLogBoundAcceptsRFC3339(t *testing.T) {
	ts := time.Date(2026, 2, 19, 10, 11, 12, 0, time.UTC)
	api, cmp, err := normalizeLogBound(ts.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := ts.Unix()
	if api != strconv.FormatInt(want, 10) {
		t.Fatalf("unexpected api value %q", api)
	}
	if cmp != want {
		t.Fatalf("expected compare unix %d, got %d", want, cmp)
	}
}

func TestNormalizeLogBoundRejectsInvalid(t *testing.T) {
	if _, _, err := normalizeLogBound("2026/02/19"); err == nil {
		t.Fatalf("expected error for invalid format")
	}
}

func TestBuildLogParamsRejectsInvertedRange(t *testing.T) {
	_, _, _, err := buildLogParams(logOptions{
		Since: "2026-02-20",
		Until: "2026-02-19",
	})
	if err == nil {
		t.Fatalf("expected range error")
	}
}

func TestBuildLogParamsSetsBoundsAndLimit(t *testing.T) {
	params, since, until, err := buildLogParams(logOptions{
		Since: "2026-02-01",
		Until: "2026-02-19",
		Limit: 321,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if since != "2026-02-01" || until != "2026-02-19" {
		t.Fatalf("unexpected bounds: since=%q until=%q", since, until)
	}
	if got := params.Get("limit"); got != "321" {
		t.Fatalf("expected limit=321, got %q", got)
	}
	if got := params.Get("since"); got != "2026-02-01" {
		t.Fatalf("expected since set, got %q", got)
	}
	if got := params.Get("until"); got != "2026-02-19" {
		t.Fatalf("expected until set, got %q", got)
	}
}
