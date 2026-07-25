package browsertools_test

import (
	"encoding/json"
	"testing"

	"github.com/strelov1/freehire/internal/browsertools"
)

// fakeSocket records what the hub handed it.
type fakeSocket struct{ sent [][]byte }

func (f *fakeSocket) Send(frame []byte) error {
	f.sent = append(f.sent, frame)
	return nil
}

func (f *fakeSocket) last() string {
	if len(f.sent) == 0 {
		return ""
	}
	return string(f.sent[len(f.sent)-1])
}

func TestForwardCarriesCallAndResultBetweenTheSameUsersEnds(t *testing.T) {
	hub := browsertools.New()
	harness, extension := &fakeSocket{}, &fakeSocket{}
	hub.Join(7, browsertools.RoleHarness, harness)
	hub.Join(7, browsertools.RoleExtension, extension)

	hub.Forward(7, browsertools.RoleHarness, []byte(`{"id":"c1","tool":"read_form"}`))
	hub.Forward(7, browsertools.RoleExtension, []byte(`{"id":"c1","result":{"fields":[]}}`))

	if got := extension.last(); got != `{"id":"c1","tool":"read_form"}` {
		t.Fatalf("extension got %q, want the call verbatim", got)
	}
	if got := harness.last(); got != `{"id":"c1","result":{"fields":[]}}` {
		t.Fatalf("harness got %q, want the result verbatim", got)
	}
}

func TestForwardNeverReachesAnotherUsersExtension(t *testing.T) {
	hub := browsertools.New()
	harnessA, extensionB := &fakeSocket{}, &fakeSocket{}
	hub.Join(1, browsertools.RoleHarness, harnessA)
	hub.Join(2, browsertools.RoleExtension, extensionB)

	hub.Forward(1, browsertools.RoleHarness, []byte(`{"id":"c1","tool":"read_form"}`))

	if len(extensionB.sent) != 0 {
		t.Fatalf("user 2's extension received %d frames from user 1's harness", len(extensionB.sent))
	}
	if got := errorOf(t, harnessA.last()); got == "" {
		t.Fatal("user 1's harness got no answer; it would hang")
	}
}

func TestCallWithNoExtensionIsAnsweredWithAnErrorResult(t *testing.T) {
	hub := browsertools.New()
	harness := &fakeSocket{}
	hub.Join(7, browsertools.RoleHarness, harness)

	hub.Forward(7, browsertools.RoleHarness, []byte(`{"id":"c9","tool":"read_form"}`))

	var answer struct {
		ID    string `json:"id"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(harness.last()), &answer); err != nil {
		t.Fatalf("answer is not JSON: %v", err)
	}
	if answer.ID != "c9" {
		t.Fatalf("answer id = %q, want the call's id so the harness can correlate it", answer.ID)
	}
	if answer.Error == "" {
		t.Fatal("answer carries no error; the harness cannot tell it failed")
	}
}

func TestResultWithNoHarnessIsDroppedRatherThanEchoed(t *testing.T) {
	hub := browsertools.New()
	extension := &fakeSocket{}
	hub.Join(7, browsertools.RoleExtension, extension)

	hub.Forward(7, browsertools.RoleExtension, []byte(`{"id":"c1","result":{}}`))

	if len(extension.sent) != 0 {
		t.Fatalf("extension was sent %d frames; its own result echoed back", len(extension.sent))
	}
}

func TestLeaveDetachesOnlyTheConnectionThatLeft(t *testing.T) {
	hub := browsertools.New()
	harness := &fakeSocket{}
	hub.Join(7, browsertools.RoleHarness, harness)

	first := &fakeSocket{}
	leaveFirst := hub.Join(7, browsertools.RoleExtension, first)
	second := &fakeSocket{}
	hub.Join(7, browsertools.RoleExtension, second)

	// The displaced panel closes after its replacement attached.
	leaveFirst()
	hub.Forward(7, browsertools.RoleHarness, []byte(`{"id":"c1","tool":"read_form"}`))

	if len(second.sent) != 1 {
		t.Fatalf("live extension got %d frames, want 1 — the displaced one evicted it", len(second.sent))
	}
	if len(first.sent) != 0 {
		t.Fatalf("displaced extension still received %d frames", len(first.sent))
	}
}

func TestParseRoleAcceptsOnlyTheTwoEnds(t *testing.T) {
	for _, name := range []string{"extension", "harness"} {
		if _, ok := browsertools.ParseRole(name); !ok {
			t.Fatalf("role %q rejected", name)
		}
	}
	if _, ok := browsertools.ParseRole("observer"); ok {
		t.Fatal("an unknown role was accepted")
	}
}

func errorOf(t *testing.T, frame string) string {
	t.Helper()
	var answer struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(frame), &answer); err != nil {
		return ""
	}
	return answer.Error
}
