package sources

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestInploiReadsStructuredSearchCredentials(t *testing.T) {
	fixture := inploiSearchFixture(t, "pk_live_example_key_1", "248")
	fake := &inploiFake{pages: map[string]string{inploiSearchURL("compass"): fixture}, final: map[string]string{inploiSearchURL("compass"): "https://jobs.compass-group.co.uk/search"}}
	key, segment, host, err := (inploi{http: fake}).creds(context.Background(), "compass")
	if err != nil || key != "pk_live_example_key_1" || segment != "248" || host != "jobs.compass-group.co.uk" {
		t.Fatalf("credentials=%q/%q host=%q err=%v", key, segment, host, err)
	}
}

func TestInploiRejectsUnrelatedOrIncompleteSearchConfig(t *testing.T) {
	for _, tc := range []struct {
		name, old   string
		replacement any
	}{
		{"wrong filter", "segment_ids", "department_ids"},
		{"wrong filter schema", "jobSearchFilter", "otherComponent"},
		{"missing default", "defaultValue", "unrelatedValue"},
		{"wrong segment type", "248", true},
		{"numeric segment is not a string", "248", 248},
		{"missing page schema", "page", "otherPage"},
		{"missing SDK schema", "sdk", "otherSDK"},
		{"invalid segment", "248", "0"},
		{"invalid value", "pk_test_inploi_fixture_123456", "invalid"},
		{"wrong key property", "apiKey", "secretKey"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := inploiMutateFixture(t, tc.old, tc.replacement)
			fake := &inploiFake{pages: map[string]string{inploiSearchURL("compass"): fixture}, final: map[string]string{inploiSearchURL("compass"): "https://jobs.compass-group.co.uk/search"}}
			if _, _, _, err := (inploi{http: fake}).creds(context.Background(), "compass"); err == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
}

func inploiSearchFixture(t *testing.T, key, segment string) string {
	t.Helper()
	fixture := inploiMutateFixture(t, "pk_test_inploi_fixture_123456", key)
	return strings.ReplaceAll(fixture, `\"248\"`, `\"`+segment+`\"`)
}

func inploiMutateFixture(t *testing.T, old string, replacement any) string {
	t.Helper()
	b, err := os.ReadFile("testdata/inploi/compass-search-stream.html")
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "window.__reactRouterContext.streamController.enqueue("
	text := string(b)
	at := strings.Index(text, prefix)
	var payload string
	if err := json.NewDecoder(strings.NewReader(text[at+len(prefix):])).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	var table []json.RawMessage
	if err := json.Unmarshal([]byte(payload), &table); err != nil {
		t.Fatal(err)
	}
	for i, cell := range table {
		var value string
		if json.Unmarshal(cell, &value) == nil && value == old {
			table[i], err = json.Marshal(replacement)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	encoded, err := json.Marshal(table)
	if err != nil {
		t.Fatal(err)
	}
	quoted, err := json.Marshal(string(encoded))
	if err != nil {
		t.Fatal(err)
	}
	return "<script>" + prefix + string(quoted) + ");</script>"
}
