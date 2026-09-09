package autofillagent_test

import (
	"context"
	"encoding/json"
	"errors"
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
// This is that missing check, in the only form that can span it: one file, produced by
// the real send path on this side and parsed by the real argument reader on the other.
// A change to what this side sends fails here with instructions to regenerate; the
// regenerated fixture then fails the extension's test, which asserts both the values it
// reads AND that it recognises every key present — so a field added here cannot be
// silently ignored there, which is exactly how the scope was lost.
const fillCallFixture = "../../../extension/lib/tools/testdata/fill-simple-call.json"

// capturingTools records the tool call Run issues rather than answering from a canned
// form. Deliberately driving Run instead of marshalling a Fill directly: the args
// wrapper (`{"fills": …}`) and the tool NAME are part of what goes on the wire, and a
// fixture assembled by hand beside them would not notice either being renamed.
type capturingTools struct {
	fields []autofillagent.Field
	tool   string
	args   any
}

func (c *capturingTools) Call(_ context.Context, tool string, args any) (json.RawMessage, error) {
	switch tool {
	case "read_form":
		return json.Marshal(map[string]any{"fields": c.fields})
	case "fill_simple":
		c.tool, c.args = tool, args
		return json.Marshal(map[string]any{"outcomes": []map[string]string{{"label": "Email", "status": "filled"}}})
	default:
		return nil, errors.New("unknown tool: " + tool)
	}
}

// The envelope Caller.Call wraps every tool call in. Duplicated here rather than
// exported from browsertools: the envelope's own correctness is caller_test.go's
// business, and what this fixture is for is the payload.
type toolCallFrame struct {
	ID   string `json:"id"`
	Tool string `json:"tool"`
	Args any    `json:"args,omitempty"`
}

// TestFillSimpleWireFixtureIsCurrent regenerates the frame from a real Run and fails
// if the committed fixture no longer matches it.
//
// Regenerate with: UPDATE_WIRE_FIXTURE=1 go test ./internal/ai/autofillagent/
func TestFillSimpleWireFixtureIsCurrent(t *testing.T) {
	// Both scopes non-zero and different from each other, so a fixture that lost one
	// or transposed them cannot pass: frame 1 is an ATS iframe, form 2 the third form
	// inside it. A zero would be indistinguishable from an absent field.
	tools := &capturingTools{fields: []autofillagent.Field{
		{Label: "Email", Type: "email", Frame: 1, Form: 2},
	}}
	planner := plannerFunc(func(_ []autofillagent.Field, p autofillagent.Profile) ([]autofillagent.Fill, error) {
		return []autofillagent.Fill{{Label: "Email", Value: p["email"]}}, nil
	})
	if _, err := autofillagent.Run(context.Background(), tools, planner, profile()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if tools.args == nil {
		t.Fatal("Run issued no fill_simple call, so there is no frame to record")
	}

	want, err := json.MarshalIndent(toolCallFrame{ID: "call-1", Tool: tools.tool, Args: tools.args}, "", "  ")
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
