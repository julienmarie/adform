package ci

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestQualityWorkflowContract(t *testing.T) {
	data, err := os.ReadFile("../../.github/workflows/quality.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		On          map[string]any    `yaml:"on"`
		Permissions map[string]string `yaml:"permissions"`
		Jobs        map[string]struct {
			Steps []struct {
				Uses string         `yaml:"uses"`
				Run  string         `yaml:"run"`
				With map[string]any `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatalf("parse quality workflow: %v", err)
	}
	if _, ok := workflow.On["pull_request"]; !ok {
		t.Error("quality workflow must run on pull_request")
	}
	push, ok := workflow.On["push"].(map[string]any)
	if !ok || !containsString(push["branches"], "main") {
		t.Error("quality workflow must run on pushes to main")
	}
	if len(workflow.Permissions) != 1 || workflow.Permissions["contents"] != "read" {
		t.Fatalf("permissions = %#v, want contents: read only", workflow.Permissions)
	}

	job, ok := workflow.Jobs["quality"]
	if !ok {
		t.Fatal("quality job is required")
	}
	uses := make(map[string]map[string]any)
	var commands strings.Builder
	for _, step := range job.Steps {
		if step.Uses != "" {
			uses[step.Uses] = step.With
		}
		commands.WriteString(step.Run)
		commands.WriteByte('\n')
	}
	checkout := uses["actions/checkout@v4"]
	if checkout == nil || checkout["persist-credentials"] != false {
		t.Fatalf("checkout must use actions/checkout@v4 with persist-credentials: false")
	}
	setup := uses["actions/setup-go@v5"]
	if setup == nil || setup["go-version-file"] != "go.mod" || setup["cache"] != true {
		t.Fatalf("setup-go must use go.mod and dependency caching: %#v", setup)
	}
	if uses["actions/upload-artifact@v4"] == nil {
		t.Fatal("checksum evidence must use actions/upload-artifact@v4")
	}

	run := commands.String()
	for _, required := range []string{
		"gofmt -l .", "go test ./...", "go vet ./...", "CGO_ENABLED=0",
		"GOOS=linux", "GOARCH=amd64", "-trimpath", "-buildvcs=false",
		"-buildid=", "./cmd/adform-reader", "cmp ", "sha256sum ",
	} {
		if !strings.Contains(run, required) {
			t.Errorf("quality commands missing %q", required)
		}
	}
	if strings.Count(run, "go build ") != 2 {
		t.Fatalf("go build count = %d, want two independent builds", strings.Count(run, "go build "))
	}

	text := string(data)
	for _, forbidden := range []string{
		"secrets.", "META_ACCESS_TOKEN", "META_API_KEY", "adform apply",
		"docker/login-action", "docker/build-push-action", "packages: write", "contents: write",
	} {
		if strings.Contains(text, forbidden) {
			t.Errorf("quality workflow contains forbidden surface %q", forbidden)
		}
	}
}

func containsString(value any, want string) bool {
	items, ok := value.([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
