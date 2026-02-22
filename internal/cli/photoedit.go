package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"adform/internal/workspace"
)

type photoeditOptions struct {
	Account          string
	Root             string
	Image            string
	Prompt           string
	Out              string
	Format           string
	Endpoint         string
	APIKey           string
	Timeout          time.Duration
	RemoveBackground bool
	Seed             int64
	JSON             bool
}

type photoEditErrorEnvelope struct {
	Error            string `json:"error"`
	ErrorCode        string `json:"errorCode"`
	ErrorDescription string `json:"errorDescription"`
	Message          string `json:"message"`
}

func runPhotoEdit(_ context.Context, args []string, stdout, stderr io.Writer) int {
	opts := photoeditOptions{}
	fs := flag.NewFlagSet("photoedit", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&opts.Account, "account", "", "Optional account name (used to resolve platforms.photoroom.api_key from <account>/accounts.yml)")
	fs.StringVar(&opts.Root, "root", ".", "Repo root (used for relative paths)")
	fs.StringVar(&opts.Image, "image", "", "Input image path (required)")
	fs.StringVar(&opts.Prompt, "prompt", "", "Edit prompt describing desired changes (required)")
	fs.StringVar(&opts.Out, "out", "", "Output image path (default: <image>.edited.<format>)")
	fs.StringVar(&opts.Format, "format", "png", "Export image format: png|jpg|jpeg|webp")
	fs.StringVar(&opts.Endpoint, "endpoint", "https://image-api.photoroom.com/v2/edit", "PhotoRoom edit API endpoint")
	fs.StringVar(&opts.APIKey, "api-key", "", "PhotoRoom API key override (otherwise PHOTOROOM_API_KEY or accounts.yml)")
	fs.DurationVar(&opts.Timeout, "timeout", 2*time.Minute, "HTTP request timeout")
	fs.BoolVar(&opts.RemoveBackground, "remove-background", false, "Remove subject background before applying edit")
	fs.Int64Var(&opts.Seed, "seed", 0, "Optional seed for deterministic edits")
	fs.BoolVar(&opts.JSON, "json", false, "Machine-readable JSON output")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	imagePathRaw := strings.TrimSpace(opts.Image)
	if imagePathRaw == "" {
		fmt.Fprintln(stderr, "error: --image is required")
		return 1
	}
	prompt := strings.TrimSpace(opts.Prompt)
	if prompt == "" {
		fmt.Fprintln(stderr, "error: --prompt is required")
		return 1
	}
	format, err := normalizePhotoEditFormat(opts.Format)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if opts.Timeout <= 0 {
		fmt.Fprintln(stderr, "error: --timeout must be > 0")
		return 1
	}
	endpoint, err := normalizePhotoEditEndpoint(opts.Endpoint)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	apiKeySource := "flag:--api-key"
	apiKey := strings.TrimSpace(opts.APIKey)
	if apiKey == "" {
		apiKey, apiKeySource, err = workspace.ResolvePhotoRoomToken(opts.Root, opts.Account)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
	}

	imagePath := imagePathRaw
	if !filepath.IsAbs(imagePath) {
		imagePath = filepath.Join(opts.Root, imagePath)
	}
	imageBytes, err := os.ReadFile(imagePath)
	if err != nil {
		fmt.Fprintf(stderr, "error: read --image: %v\n", err)
		return 1
	}
	outputPath, err := resolvePhotoEditOutputPath(opts.Root, opts.Out, imagePathRaw, format)
	if err != nil {
		fmt.Fprintf(stderr, "error: resolve --out: %v\n", err)
		return 1
	}

	body, contentType, err := buildPhotoEditMultipart(filepath.Base(imagePath), imageBytes, prompt, format, opts.RemoveBackground, opts.Seed)
	if err != nil {
		fmt.Fprintf(stderr, "error: build request: %v\n", err)
		return 1
	}
	respBody, respContentType, statusCode, err := runPhotoEditRequest(endpoint, apiKey, contentType, body, opts.Timeout)
	if err != nil {
		fmt.Fprintf(stderr, "error: call photoroom: %v\n", err)
		return 1
	}
	if statusCode < 200 || statusCode >= 300 {
		fmt.Fprintf(stderr, "error: photoroom status %d: %s\n", statusCode, summarizePhotoEditError(respBody))
		return 1
	}
	if strings.Contains(strings.ToLower(respContentType), "json") {
		fmt.Fprintf(stderr, "error: photoroom returned JSON instead of image: %s\n", summarizePhotoEditError(respBody))
		return 1
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		fmt.Fprintf(stderr, "error: create output dir: %v\n", err)
		return 1
	}
	if err := os.WriteFile(outputPath, respBody, 0o644); err != nil {
		fmt.Fprintf(stderr, "error: write output: %v\n", err)
		return 1
	}

	payload := map[string]any{
		"account":           opts.Account,
		"image":             imagePathRaw,
		"output":            outputPath,
		"prompt":            prompt,
		"format":            format,
		"endpoint":          endpoint,
		"remove_background": opts.RemoveBackground,
		"seed":              opts.Seed,
		"response_bytes":    len(respBody),
		"content_type":      respContentType,
		"api_key_source":    apiKeySource,
		"edited_at":         time.Now().UTC().Format(time.RFC3339),
	}
	if opts.JSON {
		b, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Fprintln(stdout, string(b))
		return 0
	}

	fmt.Fprintln(stdout, "photo edit complete")
	fmt.Fprintf(stdout, "- in: %s\n", imagePathRaw)
	fmt.Fprintf(stdout, "- out: %s\n", outputPath)
	fmt.Fprintf(stdout, "- format: %s\n", format)
	fmt.Fprintf(stdout, "- bytes: %d\n", len(respBody))
	return 0
}

func normalizePhotoEditFormat(raw string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "png", "jpg", "jpeg", "webp":
		return v, nil
	default:
		return "", fmt.Errorf("invalid --format %q (expected png|jpg|jpeg|webp)", raw)
	}
}

func normalizePhotoEditEndpoint(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", fmt.Errorf("missing --endpoint")
	}
	u, err := neturl.Parse(v)
	if err != nil {
		return "", err
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", fmt.Errorf("unsupported --endpoint scheme %q (expected http/https)", u.Scheme)
	}
	if strings.TrimSpace(u.Host) == "" {
		return "", fmt.Errorf("invalid --endpoint host")
	}
	return v, nil
}

func resolvePhotoEditOutputPath(root, out, imagePathRaw, format string) (string, error) {
	v := strings.TrimSpace(out)
	if v == "" {
		imageOut := imagePathRaw
		if imageOut == "" {
			imageOut = "image"
		}
		ext := filepath.Ext(imageOut)
		stem := strings.TrimSuffix(imageOut, ext)
		if stem == "" {
			stem = imageOut
		}
		v = stem + ".edited." + format
	}
	if !filepath.IsAbs(v) {
		v = filepath.Join(root, v)
	}
	return v, nil
}

func buildPhotoEditMultipart(imageFilename string, imageBytes []byte, prompt, format string, removeBackground bool, seed int64) ([]byte, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	if err := w.WriteField("describeAnyChange.mode", "ai.auto"); err != nil {
		return nil, "", err
	}
	if err := w.WriteField("describeAnyChange.prompt", prompt); err != nil {
		return nil, "", err
	}
	if err := w.WriteField("removeBackground", strconv.FormatBool(removeBackground)); err != nil {
		return nil, "", err
	}
	if err := w.WriteField("export.format", format); err != nil {
		return nil, "", err
	}
	if seed != 0 {
		if err := w.WriteField("describeAnyChange.seed", strconv.FormatInt(seed, 10)); err != nil {
			return nil, "", err
		}
	}
	part, err := w.CreateFormFile("imageFile", imageFilename)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(imageBytes); err != nil {
		return nil, "", err
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), w.FormDataContentType(), nil
}

func runPhotoEditRequest(endpoint, apiKey, contentType string, body []byte, timeout time.Duration) ([]byte, string, int, error) {
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, "", 0, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "image/*")

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", 0, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", resp.StatusCode, err
	}
	return respBody, resp.Header.Get("Content-Type"), resp.StatusCode, nil
}

func summarizePhotoEditError(body []byte) string {
	if len(body) == 0 {
		return "empty response body"
	}
	var env photoEditErrorEnvelope
	if err := json.Unmarshal(body, &env); err == nil {
		parts := []string{}
		if strings.TrimSpace(env.ErrorCode) != "" {
			parts = append(parts, env.ErrorCode)
		}
		if strings.TrimSpace(env.ErrorDescription) != "" {
			parts = append(parts, env.ErrorDescription)
		}
		if strings.TrimSpace(env.Message) != "" {
			parts = append(parts, env.Message)
		}
		if strings.TrimSpace(env.Error) != "" {
			parts = append(parts, env.Error)
		}
		if len(parts) > 0 {
			return strings.Join(parts, " | ")
		}
	}
	text := strings.TrimSpace(string(body))
	if len(text) > 800 {
		text = text[:800] + "..."
	}
	return text
}
