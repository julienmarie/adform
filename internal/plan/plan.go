package plan

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"adform/internal/config"
	"adform/internal/render"
	"adform/internal/state"
)

type OperationType string

const (
	OpCreate      OperationType = "create"
	OpUpdate      OperationType = "update"
	OpReplace     OperationType = "replace"
	OpPauseOrphan OperationType = "pause-orphan"
	OpDriftOnly   OperationType = "drift-only"
	OpNoop        OperationType = "noop"
)

type Operation struct {
	ID        string        `json:"id"`
	Kind      string        `json:"kind"`
	Key       string        `json:"key"`
	Action    OperationType `json:"action"`
	Reason    string        `json:"reason"`
	Hash      string        `json:"hash,omitempty"`
	DependsOn []string      `json:"depends_on,omitempty"`
}

type Plan struct {
	Account     string      `json:"account"`
	GeneratedAt string      `json:"generated_at"`
	Operations  []Operation `json:"operations"`
	Summary     Summary     `json:"summary"`
}

type Summary struct {
	Create      int `json:"create"`
	Update      int `json:"update"`
	Replace     int `json:"replace"`
	PauseOrphan int `json:"pause_orphan"`
	DriftOnly   int `json:"drift_only"`
	Noop        int `json:"noop"`
}

type DesiredResource struct {
	Kind      string
	Key       string
	Hash      string
	DependsOn []string
}

func Build(account string, bundle config.Bundle, st *state.Store) (Plan, error) {
	resolved := render.Resolve(bundle)
	desired, err := desiredResources(resolved)
	if err != nil {
		return Plan{}, err
	}

	ops := make([]Operation, 0, len(desired))
	desiredSet := make(map[string]DesiredResource, len(desired))
	for _, d := range desired {
		desiredSet[d.Kind+":"+d.Key] = d

		row, err := st.GetResource(account, d.Kind, d.Key)
		if err != nil {
			return Plan{}, err
		}
		op := Operation{
			ID:        d.Kind + ":" + d.Key,
			Kind:      d.Kind,
			Key:       d.Key,
			Hash:      d.Hash,
			DependsOn: d.DependsOn,
		}
		switch {
		case row == nil:
			op.Action = OpCreate
			op.Reason = "missing in state"
		case row.LastAppliedHash == d.Hash:
			op.Action = OpNoop
			op.Reason = "desired matches last applied hash"
		case d.Kind == "creative":
			op.Action = OpReplace
			op.Reason = "creative treated as immutable"
		default:
			op.Action = OpUpdate
			op.Reason = "hash changed"
		}
		ops = append(ops, op)
	}

	rows, err := st.ListResources(account)
	if err != nil {
		return Plan{}, err
	}
	for _, row := range rows {
		id := row.Kind + ":" + row.LogicalKey
		if _, ok := desiredSet[id]; ok {
			continue
		}
		ops = append(ops, Operation{
			ID:     id,
			Kind:   row.Kind,
			Key:    row.LogicalKey,
			Action: OpPauseOrphan,
			Reason: "managed resource missing from config",
		})
	}

	sort.SliceStable(ops, func(i, j int) bool {
		ki := kindOrder(ops[i].Kind)
		kj := kindOrder(ops[j].Kind)
		if ki != kj {
			return ki < kj
		}
		return strings.Compare(ops[i].ID, ops[j].ID) < 0
	})

	plan := Plan{
		Account:     account,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Operations:  ops,
	}
	for _, op := range ops {
		switch op.Action {
		case OpCreate:
			plan.Summary.Create++
		case OpUpdate:
			plan.Summary.Update++
		case OpReplace:
			plan.Summary.Replace++
		case OpPauseOrphan:
			plan.Summary.PauseOrphan++
		case OpDriftOnly:
			plan.Summary.DriftOnly++
		case OpNoop:
			plan.Summary.Noop++
		}
	}
	return plan, nil
}

func desiredResources(bundle config.Bundle) ([]DesiredResource, error) {
	out := make([]DesiredResource, 0)

	assetKeys := sortedKeys(bundle.Assets)
	for _, key := range assetKeys {
		asset := bundle.Assets[key]
		h, err := render.Hash(asset)
		if err != nil {
			return nil, fmt.Errorf("hash asset %s: %w", key, err)
		}
		kind := "asset_image"
		if asset.Type == "video" {
			kind = "asset_video"
		}
		out = append(out, DesiredResource{Kind: kind, Key: key, Hash: h})
	}

	campaignKeys := sortedKeys(bundle.Campaigns)
	for _, key := range campaignKeys {
		c := bundle.Campaigns[key]
		payload := map[string]any{
			"name":                  c.Name,
			"objective":             c.Objective,
			"status":                strings.ToUpper(c.Status),
			"special_ad_categories": c.SpecialAdCategories,
		}
		h, err := render.Hash(payload)
		if err != nil {
			return nil, fmt.Errorf("hash campaign %s: %w", key, err)
		}
		out = append(out, DesiredResource{Kind: "campaign", Key: key, Hash: h})
	}

	creativeKeys := sortedKeys(bundle.Creatives)
	for _, key := range creativeKeys {
		c := bundle.Creatives[key]
		h, err := render.Hash(c)
		if err != nil {
			return nil, fmt.Errorf("hash creative %s: %w", key, err)
		}
		out = append(out, DesiredResource{Kind: "creative", Key: key, Hash: h})
	}

	audienceKeys := sortedKeys(bundle.Audiences)
	for _, key := range audienceKeys {
		a := bundle.Audiences[key]
		h, err := render.Hash(a)
		if err != nil {
			return nil, fmt.Errorf("hash audience %s: %w", key, err)
		}
		out = append(out, DesiredResource{Kind: "audience", Key: key, Hash: h})
	}
	catalogKeys := sortedKeys(bundle.Catalogs)
	for _, key := range catalogKeys {
		c := bundle.Catalogs[key]
		h, err := render.Hash(c)
		if err != nil {
			return nil, fmt.Errorf("hash catalog %s: %w", key, err)
		}
		out = append(out, DesiredResource{Kind: "catalog", Key: key, Hash: h})
	}

	adsetKeys := sortedKeys(bundle.Adsets)
	for _, key := range adsetKeys {
		a := bundle.Adsets[key]
		h, err := render.Hash(a)
		if err != nil {
			return nil, fmt.Errorf("hash adset %s: %w", key, err)
		}
		out = append(out, DesiredResource{
			Kind:      "adset",
			Key:       key,
			Hash:      h,
			DependsOn: []string{"campaign:" + a.CampaignKey},
		})
	}

	adKeys := sortedKeys(bundle.Ads)
	for _, key := range adKeys {
		a := bundle.Ads[key]
		h, err := render.Hash(a)
		if err != nil {
			return nil, fmt.Errorf("hash ad %s: %w", key, err)
		}
		out = append(out, DesiredResource{
			Kind:      "ad",
			Key:       key,
			Hash:      h,
			DependsOn: []string{"adset:" + a.AdsetKey, "creative:" + a.CreativeKey},
		})
	}

	return out, nil
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func kindOrder(kind string) int {
	switch kind {
	case "asset_image", "asset_video":
		return 1
	case "campaign":
		return 2
	case "creative":
		return 3
	case "audience":
		return 4
	case "catalog":
		return 5
	case "adset":
		return 6
	case "ad":
		return 7
	default:
		return 100
	}
}
