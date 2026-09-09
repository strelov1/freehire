package autofillagent_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/strelov1/freehire/internal/ai/autofillagent"
)

// fillCallFixture is the `fill_simple` frame this package emits, kept as a file the
// EXTENSION's own test reads back (extension/lib/tools/executor.test.ts).
//
// It exists because of how the frame scope was lost. Fill has carried `frame` since
// frame scoping was introduced, and TestRunRoutesSameLabeledFieldsInDifferentFrames-
// Independently asserted that it did — while the extension's readFills destructured
// only {label, value} and dropped it, so every agent-planned fill arrived unscoped and
// was broadcast to every frame and matched by label alone. The Go test was green
// because it checked the SENDER. Nothing checked that the receiver read what was sent,
// and the two ends are in different languages, so no compiler spans the gap.
//
// This is that missing check, in the only form that can span it: one file, written from
// the real struct on this side and parsed by the real argument reader on the other. A
// change to Fill's JSON fails here with instructions to regenerate; the regenerated
// fixture then fails the extension's test if its reader cannot handle the new shape.
const fillCallFixture = "../../../extension/lib/tools/testdata/fill-simple-call.json"

// The envelope Caller.Call wraps every tool call in. Duplicated here rather than
// exported from browsertools: the envelope's own correctness is caller_test.go's
// business, and what this fixture is for is the ARGS — one Fill, marshalled by the
// struct that actually goes on the wire.
type toolCallFrame struct {
	ID   string `json:"id"`
	Tool string `json:"tool"`
	Args any    `json:"args,omitempty"`
}

// TestFillSimpleWireFixtureIsCurrent regenerates the frame from the live struct and
// fails if the committed fixture no longer matches it.
//
// Regenerate with: UPDATE_WIRE_FIXTURE=1 go test ./internal/ai/autofillagent/
func TestFillSimpleWireFixtureIsCurrent(t *testing.T) {
	// Both scopes non-zero and different from each other, so a fixture that lost one
	// or transposed them cannot pass: frame 1 is an ATS iframe, form 2 the third form
	// inside it. A zero would be indistinguishable from an absent field.
	fills := []autofillagent.Fill{{Label: "Email", Value: "ilya@example.com", Frame: 1, Form: 2}}
	frame := toolCallFrame{ID: "call-1", Tool: "fill_simple", Args: map[string]any{"fills": fills}}

	want, err := json.MarshalIndent(frame, "", "  ")
	if err != nil {
		t.Fatalf("marshal frame: %v", err)
	}
	want = append(want, '\n')

	if updateWireFixture() {
		if err := os.MkdirAll(filepath.Dir(fillCallFixture), 0o755); err != nil {
			t.Fatalf("create fixture dir: %v", err)
		}
		if err := os.WriteFile(fillCallFixture, want, 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		t.Log("fixture regenerated; re-run the extension's tests against it")
		return
	}

	got, err := os.ReadFile(fillCallFixture)
	if err != nil {
		t.Fatalf("read fixture: %v\nregenerate with: UPDATE_WIRE_FIXTURE=1 go test ./internal/ai/autofillagent/", err)
	}
	if string(got) != string(want) {
		t.Fatalf("the fill_simple frame no longer matches the fixture the extension's test reads.\n got: %s\nwant: %s\nregenerate with: UPDATE_WIRE_FIXTURE=1 go test ./internal/ai/autofillagent/", got, want)
	}
}

// updateWireFixture reports whether the run was asked to rewrite the fixture.
//
// An environment variable and not a flag: `go test` parses the command line itself
// and rejects a flag it was not given, so an unregistered one never reaches this
// code, and registering it would add a flag to every test binary in the module for
// the sake of one test.
func updateWireFixture() bool {
	return os.Getenv("UPDATE_WIRE_FIXTURE") != ""
}
