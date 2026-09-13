package search

import (
	"encoding/json"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

// The provider allow-list itself now lives in jobview (see
// jobview.AutoApplyProviders and its own TestAutoApplyProviders_ExactExpectedSet)
// since FromJob serves the signal on the public wire shape via the embedded
// jobview.Job, not as a document-only field. These tests cover FromJob's own
// wiring: that the value jobview computed reaches the document JSON unchanged.
func TestFromJob_AutoApplyAvailable(t *testing.T) {
	for _, provider := range []string{"greenhouse", "lever", "ashby", "workable"} {
		doc, err := FromJob(db.Job{ID: 1, PublicSlug: "s", Source: provider})
		if err != nil {
			t.Fatalf("FromJob(%q): %v", provider, err)
		}
		if !doc.AutoApplyAvailable {
			t.Errorf("source %q: AutoApplyAvailable = false, want true", provider)
		}
	}
}

func TestFromJob_AutoApplyAvailable_NotEligibleProvider(t *testing.T) {
	for _, provider := range []string{"recruitee", "djinni", ""} {
		doc, err := FromJob(db.Job{ID: 1, PublicSlug: "s", Source: provider})
		if err != nil {
			t.Fatalf("FromJob(%q): %v", provider, err)
		}
		if doc.AutoApplyAvailable {
			t.Errorf("source %q: AutoApplyAvailable = true, want false", provider)
		}
	}
}

// marshalToMap round-trips a JobDocument through JSON into a generic map, so a
// test can assert a key's presence (or absence) rather than just its value —
// `omitempty` drops the key entirely, which a typed struct field can't observe.
func marshalToMap(t *testing.T, doc JobDocument) map[string]any {
	t.Helper()
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	return raw
}

func TestFromJob_AutoApplyAvailable_OmittedFromJSONWhenFalse(t *testing.T) {
	doc, err := FromJob(db.Job{ID: 1, PublicSlug: "s", Source: "recruitee"})
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	if _, present := marshalToMap(t, doc)["auto_apply_available"]; present {
		t.Error("auto_apply_available key present in JSON when false, want omitted")
	}
}

func TestFromJob_AutoApplyAvailable_PresentInJSONWhenTrue(t *testing.T) {
	doc, err := FromJob(db.Job{ID: 1, PublicSlug: "s", Source: "greenhouse"})
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	if v, present := marshalToMap(t, doc)["auto_apply_available"]; !present || v != true {
		t.Errorf("auto_apply_available = %v, present=%v, want true, present=true", v, present)
	}
}
