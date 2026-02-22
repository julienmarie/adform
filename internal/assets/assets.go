package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"adform/internal/config"
	"adform/internal/render"
	"adform/internal/state"
	"adform/internal/workspace"
	"gopkg.in/yaml.v3"
)

type UploadOptions struct {
	Root        string
	Account     string
	AdAccountID string
	Type        string
	Path        string
	Uploader    Uploader
}

type Uploader interface {
	UploadImage(adAccountID, filePath string) (string, error)
	UploadVideo(adAccountID, filePath string) (string, error)
}

type UploadItem struct {
	Key      string `json:"key"`
	Kind     string `json:"kind"`
	Path     string `json:"path"`
	SHA256   string `json:"sha256"`
	MetaID   string `json:"meta_id"`
	Deduped  bool   `json:"deduped"`
	Uploaded bool   `json:"uploaded"`
	Message  string `json:"message"`
}

type UploadResult struct {
	ManifestPath string       `json:"manifest_path"`
	Uploaded     int          `json:"uploaded"`
	Deduped      int          `json:"deduped"`
	Skipped      int          `json:"skipped"`
	Errors       int          `json:"errors"`
	Items        []UploadItem `json:"items"`
}

type VerifyItem struct {
	Key      string `json:"key"`
	Type     string `json:"type"`
	File     string `json:"file"`
	OK       bool   `json:"ok"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
	Message  string `json:"message"`
}

type VerifyResult struct {
	Checked int          `json:"checked"`
	Failed  int          `json:"failed"`
	Items   []VerifyItem `json:"items"`
}

type GCItem struct {
	Kind string `json:"kind"`
	Key  string `json:"key"`
}

type GCResult struct {
	DryRun  bool     `json:"dry_run"`
	Deleted int      `json:"deleted"`
	Items   []GCItem `json:"items"`
}

func Upload(st *state.Store, opts UploadOptions) (UploadResult, error) {
	manifestPath := filepath.Join(workspace.ResolveMetaDir(opts.Root, opts.Account), "assets.yml")
	manifest, err := loadManifest(manifestPath)
	if err != nil {
		return UploadResult{}, err
	}
	if opts.Type != "" && opts.Type != "image" && opts.Type != "video" {
		return UploadResult{}, fmt.Errorf("--type must be image or video")
	}

	rows, err := st.ListResources(opts.Account)
	if err != nil {
		return UploadResult{}, err
	}
	hashToMeta := map[string]string{}
	for _, row := range rows {
		if row.Kind == "asset_image" || row.Kind == "asset_video" {
			dedupeHash := row.LastSeenRemoteHash
			if dedupeHash == "" {
				dedupeHash = row.LastAppliedHash
			}
			if dedupeHash != "" {
				hashToMeta[dedupeHash] = row.MetaID
			}
		}
	}

	var targetKeys map[string]struct{}
	if opts.Path != "" {
		targetKeys, err = keysByPathGlob(opts.Root, manifest, opts.Path)
		if err != nil {
			return UploadResult{}, err
		}
	}

	result := UploadResult{ManifestPath: manifestPath}
	for i := range manifest.Assets {
		a := &manifest.Assets[i]
		if opts.Type != "" && a.Type != opts.Type {
			continue
		}
		if targetKeys != nil {
			if _, ok := targetKeys[a.Key]; !ok {
				continue
			}
		}
		item := UploadItem{Key: a.Key, Kind: a.Type}
		if a.File == nil || strings.TrimSpace(*a.File) == "" {
			item.Message = "no local file, skipped"
			result.Skipped++
			result.Items = append(result.Items, item)
			continue
		}
		assetPath := *a.File
		if !filepath.IsAbs(assetPath) {
			assetPath = filepath.Join(opts.Root, assetPath)
		}
		item.Path = assetPath
		hash, err := fileSHA256(assetPath)
		if err != nil {
			item.Message = err.Error()
			result.Errors++
			result.Items = append(result.Items, item)
			continue
		}
		item.SHA256 = hash
		a.SHA256 = ptr(hash)

		metaID, deduped := hashToMeta[hash]
		if !deduped {
			if opts.Uploader != nil {
				if opts.AdAccountID == "" {
					return result, fmt.Errorf("ad account id is required for remote asset upload")
				}
				switch a.Type {
				case "image":
					metaID, err = opts.Uploader.UploadImage(opts.AdAccountID, assetPath)
				case "video":
					metaID, err = opts.Uploader.UploadVideo(opts.AdAccountID, assetPath)
				default:
					err = fmt.Errorf("unsupported asset type %q", a.Type)
				}
				if err != nil {
					item.Message = fmt.Sprintf("remote upload failed: %v", err)
					result.Errors++
					result.Items = append(result.Items, item)
					continue
				}
			} else {
				metaID = syntheticMetaID(a.Type, hash)
			}
			hashToMeta[hash] = metaID
			item.Uploaded = true
			result.Uploaded++
		} else {
			item.Deduped = true
			result.Deduped++
		}
		if a.Type == "image" {
			a.Meta.ImageHash = ptr(metaID)
			a.Meta.VideoID = nil
		} else {
			a.Meta.VideoID = ptr(metaID)
			a.Meta.ImageHash = nil
		}
		if a.Meta.Origin == "" {
			a.Meta.Origin = "local"
		}
		item.MetaID = metaID
		item.Message = "processed"
		result.Items = append(result.Items, item)
		canonicalHash, err := render.Hash(*a)
		if err != nil {
			return result, fmt.Errorf("hash asset %s: %w", a.Key, err)
		}

		kind := "asset_image"
		if a.Type == "video" {
			kind = "asset_video"
		}
		if err := st.UpsertResource(state.ResourceRow{
			AccountName:        opts.Account,
			Kind:               kind,
			LogicalKey:         a.Key,
			MetaID:             metaID,
			LastAppliedHash:    canonicalHash,
			LastSeenRemoteHash: hash,
		}); err != nil {
			return result, err
		}
	}

	sort.SliceStable(manifest.Assets, func(i, j int) bool {
		return manifest.Assets[i].Key < manifest.Assets[j].Key
	})
	if err := writeManifest(manifestPath, manifest); err != nil {
		return result, err
	}

	return result, nil
}

func List(root, account string, st *state.Store) ([]UploadItem, error) {
	manifestPath := filepath.Join(workspace.ResolveMetaDir(root, account), "assets.yml")
	manifest, err := loadManifest(manifestPath)
	if err != nil {
		return nil, err
	}
	items := make([]UploadItem, 0, len(manifest.Assets))
	for _, a := range manifest.Assets {
		item := UploadItem{Key: a.Key, Kind: a.Type}
		if a.File != nil {
			item.Path = *a.File
		}
		if a.SHA256 != nil {
			item.SHA256 = *a.SHA256
		}
		kind := "asset_image"
		if a.Type == "video" {
			kind = "asset_video"
		}
		if row, err := st.GetResource(account, kind, a.Key); err == nil && row != nil {
			item.MetaID = row.MetaID
			if item.SHA256 == "" {
				item.SHA256 = row.LastAppliedHash
			}
		}
		item.Message = a.Meta.Origin
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Key < items[j].Key })
	return items, nil
}

func Verify(root, account string, st *state.Store) (VerifyResult, error) {
	manifestPath := filepath.Join(workspace.ResolveMetaDir(root, account), "assets.yml")
	manifest, err := loadManifest(manifestPath)
	if err != nil {
		return VerifyResult{}, err
	}
	result := VerifyResult{}
	for _, a := range manifest.Assets {
		if a.File == nil || strings.TrimSpace(*a.File) == "" {
			continue
		}
		item := VerifyItem{Key: a.Key, Type: a.Type, File: *a.File, OK: true}
		result.Checked++

		assetPath := *a.File
		if !filepath.IsAbs(assetPath) {
			assetPath = filepath.Join(root, assetPath)
		}
		hash, err := fileSHA256(assetPath)
		if err != nil {
			item.OK = false
			item.Message = err.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}

		if a.SHA256 != nil && *a.SHA256 != "" && *a.SHA256 != hash {
			item.OK = false
			item.Expected = *a.SHA256
			item.Actual = hash
			item.Message = "sha mismatch against manifest"
		}

		kind := "asset_image"
		if a.Type == "video" {
			kind = "asset_video"
		}
		if row, err := st.GetResource(account, kind, a.Key); err == nil && row != nil {
			stateHash := row.LastSeenRemoteHash
			if stateHash == "" {
				stateHash = row.LastAppliedHash
			}
			if stateHash != "" && stateHash != hash {
				item.OK = false
				if item.Expected == "" {
					item.Expected = stateHash
				}
				item.Actual = hash
				item.Message = "sha mismatch against state"
			}
		}
		if item.OK {
			item.Message = "ok"
		} else {
			result.Failed++
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}

func GC(root, account string, st *state.Store, dryRun bool) (GCResult, error) {
	manifestPath := filepath.Join(workspace.ResolveMetaDir(root, account), "assets.yml")
	manifest, err := loadManifest(manifestPath)
	if err != nil {
		return GCResult{}, err
	}
	manifestKeys := map[string]struct{}{}
	for _, a := range manifest.Assets {
		manifestKeys["asset_"+a.Type+":"+a.Key] = struct{}{}
	}

	rows, err := st.ListResources(account)
	if err != nil {
		return GCResult{}, err
	}
	result := GCResult{DryRun: dryRun}
	for _, row := range rows {
		if row.Kind != "asset_image" && row.Kind != "asset_video" {
			continue
		}
		id := row.Kind + ":" + row.LogicalKey
		if _, ok := manifestKeys[id]; ok {
			continue
		}
		item := GCItem{Kind: row.Kind, Key: row.LogicalKey}
		result.Items = append(result.Items, item)
		if dryRun {
			continue
		}
		if err := st.DeleteResource(account, row.Kind, row.LogicalKey); err != nil {
			return result, err
		}
		result.Deleted++
	}
	return result, nil
}

func loadManifest(path string) (config.AssetManifest, error) {
	var manifest config.AssetManifest
	b, err := os.ReadFile(path)
	if err != nil {
		return manifest, fmt.Errorf("read assets manifest: %w", err)
	}
	if err := yaml.Unmarshal(b, &manifest); err != nil {
		return manifest, fmt.Errorf("parse assets manifest: %w", err)
	}
	return manifest, nil
}

func writeManifest(path string, manifest config.AssetManifest) error {
	b, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal assets manifest: %w", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("write assets manifest: %w", err)
	}
	return nil
}

func keysByPathGlob(root string, manifest config.AssetManifest, glob string) (map[string]struct{}, error) {
	pattern := glob
	if !filepath.IsAbs(pattern) {
		pattern = filepath.Join(root, pattern)
	}
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid --path glob: %w", err)
	}
	if len(matches) == 0 {
		return map[string]struct{}{}, nil
	}
	set := map[string]struct{}{}
	normalized := map[string]struct{}{}
	for _, m := range matches {
		normalized[filepath.Clean(m)] = struct{}{}
	}
	for _, a := range manifest.Assets {
		if a.File == nil {
			continue
		}
		p := *a.File
		if !filepath.IsAbs(p) {
			p = filepath.Join(root, p)
		}
		p = filepath.Clean(p)
		if _, ok := normalized[p]; ok {
			set[a.Key] = struct{}{}
		}
	}
	return set, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func syntheticMetaID(kind, hash string) string {
	prefix := "img"
	if kind == "video" {
		prefix = "vid"
	}
	short := hash
	if len(short) > 12 {
		short = short[:12]
	}
	return "local_" + prefix + "_" + short
}

func ptr(s string) *string {
	return &s
}
