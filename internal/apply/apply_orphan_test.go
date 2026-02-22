package apply

import (
	"net/url"
	"path/filepath"
	"testing"

	"adform/internal/plan"
	"adform/internal/state"
)

type orphanMetaStub struct {
	pausedIDs      []string
	updatedIDs     []string
	updatedPayload []url.Values
}

func (m *orphanMetaStub) CreateObject(_ string, _ url.Values) (string, error) { return "", nil }
func (m *orphanMetaStub) UploadImage(_ string, _ string) (string, error)      { return "", nil }
func (m *orphanMetaStub) UploadVideo(_ string, _ string) (string, error)      { return "", nil }
func (m *orphanMetaStub) PauseObject(id string) error {
	m.pausedIDs = append(m.pausedIDs, id)
	return nil
}
func (m *orphanMetaStub) UpdateObject(id string, params url.Values) error {
	m.updatedIDs = append(m.updatedIDs, id)
	m.updatedPayload = append(m.updatedPayload, params)
	return nil
}

func TestPauseOrphanDefaultBehavior(t *testing.T) {
	st, err := state.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err := st.UpsertResource(state.ResourceRow{
		AccountName: "acc",
		Kind:        "campaign",
		LogicalKey:  "camp",
		MetaID:      "123",
	}); err != nil {
		t.Fatal(err)
	}

	meta := &orphanMetaStub{}
	_, err = Execute("acc", st, plan.Plan{
		Operations: []plan.Operation{{ID: "campaign:camp", Kind: "campaign", Key: "camp", Action: plan.OpPauseOrphan}},
	}, Options{
		MaxOps: 1,
		DryRun: false,
		Meta:   meta,
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(meta.pausedIDs) != 1 || meta.pausedIDs[0] != "123" {
		t.Fatalf("expected PauseObject on 123, got %+v", meta.pausedIDs)
	}
	if len(meta.updatedIDs) != 0 {
		t.Fatalf("expected no UpdateObject call, got %+v", meta.updatedIDs)
	}
}

func TestPauseOrphanDeleteArchives(t *testing.T) {
	st, err := state.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err := st.UpsertResource(state.ResourceRow{
		AccountName: "acc",
		Kind:        "campaign",
		LogicalKey:  "camp",
		MetaID:      "123",
	}); err != nil {
		t.Fatal(err)
	}

	meta := &orphanMetaStub{}
	_, err = Execute("acc", st, plan.Plan{
		Operations: []plan.Operation{{ID: "campaign:camp", Kind: "campaign", Key: "camp", Action: plan.OpPauseOrphan}},
	}, Options{
		MaxOps: 1,
		DryRun: false,
		Delete: true,
		Meta:   meta,
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(meta.pausedIDs) != 0 {
		t.Fatalf("expected no PauseObject call, got %+v", meta.pausedIDs)
	}
	if len(meta.updatedIDs) != 1 || meta.updatedIDs[0] != "123" {
		t.Fatalf("expected UpdateObject on 123, got %+v", meta.updatedIDs)
	}
	if got := meta.updatedPayload[0].Get("status"); got != "ARCHIVED" {
		t.Fatalf("expected status ARCHIVED, got %q", got)
	}
}
