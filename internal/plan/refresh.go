package plan

import (
	"fmt"
	"strings"

	"adform/internal/config"
	"adform/internal/meta"
	"adform/internal/render"
	"adform/internal/state"
)

type RemoteReader interface {
	GetNode(id string, fields ...string) (map[string]any, error)
}

func RefreshFromRemote(account string, bundle config.Bundle, st *state.Store, pl Plan, remote RemoteReader, includeDrift bool) (Plan, error) {
	rows, err := st.ListResources(account)
	if err != nil {
		return pl, err
	}
	byKindKey := map[string]state.ResourceRow{}
	for _, row := range rows {
		byKindKey[row.Kind+":"+row.LogicalKey] = row
	}

	for i := range pl.Operations {
		op := &pl.Operations[i]
		row, ok := byKindKey[op.Kind+":"+op.Key]
		if !ok || row.MetaID == "" {
			continue
		}
		fields := remoteFields(op.Kind)
		if len(fields) == 0 {
			continue
		}
		remoteObj, err := remote.GetNode(row.MetaID, fields...)
		if err != nil {
			if meta.IsNotFound(err) {
				if op.Action != OpCreate {
					op.Action = OpCreate
					op.Reason = "missing remotely; re-create"
				}
				continue
			}
			op.Reason = op.Reason + " (refresh warning: " + err.Error() + ")"
			continue
		}

		hashInput := any(remoteObj)
		if op.Kind == "campaign" {
			canonical := map[string]any{
				"name":      stringValue(remoteObj["name"]),
				"objective": stringValue(remoteObj["objective"]),
				"status":    strings.ToUpper(stringValue(remoteObj["status"])),
			}
			if cats, ok := remoteObj["special_ad_categories"]; ok {
				canonical["special_ad_categories"] = cats
			} else {
				canonical["special_ad_categories"] = []string{}
			}
			hashInput = canonical
		}
		remoteHash, err := render.Hash(hashInput)
		if err != nil {
			op.Reason = op.Reason + " (refresh warning: hash failed)"
			continue
		}

		row.LastSeenRemoteHash = remoteHash
		if err := st.UpsertResource(row); err != nil {
			return pl, fmt.Errorf("persist remote hash for %s: %w", op.ID, err)
		}

		if includeDrift && op.Action == OpNoop && op.Kind == "campaign" && remoteHash != op.Hash {
			op.Action = OpDriftOnly
			op.Reason = "remote differs from desired campaign fields"
		}
	}

	pl.Summary = RecalculateSummary(pl.Operations)
	return pl, nil
}

func remoteFields(kind string) []string {
	switch kind {
	case "campaign":
		return []string{"id", "name", "objective", "status", "special_ad_categories"}
	case "adset":
		return []string{"id", "name", "status", "daily_budget", "campaign_id", "updated_time"}
	case "creative":
		return []string{"id", "name", "updated_time"}
	case "ad":
		return []string{"id", "name", "status", "updated_time"}
	default:
		return nil
	}
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}

func RecalculateSummary(ops []Operation) Summary {
	var s Summary
	for _, op := range ops {
		switch op.Action {
		case OpCreate:
			s.Create++
		case OpUpdate:
			s.Update++
		case OpReplace:
			s.Replace++
		case OpPauseOrphan:
			s.PauseOrphan++
		case OpDriftOnly:
			s.DriftOnly++
		case OpNoop:
			s.Noop++
		}
	}
	return s
}
