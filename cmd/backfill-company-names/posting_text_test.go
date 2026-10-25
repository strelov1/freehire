package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
	"github.com/strelov1/freehire/internal/platform/llm"
)

type fakeDescriptions struct {
	got  db.ListPlaceholderBoardDescriptionsParams
	rows []string
}

func (f *fakeDescriptions) ListPlaceholderBoardDescriptions(_ context.Context, arg db.ListPlaceholderBoardDescriptionsParams) ([]string, error) {
	f.got = arg
	return f.rows, nil
}

// fakeModel answers with a canned reply and records the user prompt it was sent.
type fakeModel struct {
	reply  string
	err    error
	prompt string
	calls  int
}

func (f *fakeModel) GenerateJSON(_ context.Context, _, user string, _ ...llm.GenOption) (string, error) {
	f.calls++
	f.prompt = user
	return f.reply, f.err
}

func TestPostingTextResolver(t *testing.T) {
	const board = "68b5ff7b-5642-41b3-b889-567091a75baa:19000101_000001"
	posting := "<p>Choosing where to grow your career matters. <b>San Antonio Zoo</b> is a place where you can collaborate.</p>"

	cases := []struct {
		name, reply, want string
	}{
		{"a name the text states", `{"employer": "San Antonio Zoo"}`, "San Antonio Zoo"},
		{"a name the text does not state", `{"employer": "San Antonio Zoological Society"}`, ""},
		{"the model found none", `{"employer": null}`, ""},
		{"fenced output", "```json\n{\"employer\": \"san antonio zoo\"}\n```", "San Antonio Zoo"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q := &fakeDescriptions{rows: []string{posting}}
			m := &fakeModel{reply: c.reply}
			r := postingTextResolver{q: q, model: m, source: "adp"}

			got, err := r.Name(context.Background(), board)
			if err != nil || got != c.want {
				t.Fatalf("Name = %q, %v; want %q", got, err, c.want)
			}
			if q.got.Source != "adp" || q.got.BoardPattern != externalid.BoardPattern(board) || q.got.MaxRows != postingTextSamples {
				t.Errorf("descriptions asked with %+v", q.got)
			}
			// The model reads the posting as TEXT: tags are rendered away before it is sent.
			if strings.Contains(m.prompt, "<b>") || !strings.Contains(m.prompt, "San Antonio Zoo") {
				t.Errorf("prompt = %q, want the posting as plain text", m.prompt)
			}
		})
	}
}

// A board with no stored body costs no model call; a failed call is an error, not a name.
func TestPostingTextResolverEdges(t *testing.T) {
	m := &fakeModel{reply: `{"employer": "X"}`}
	r := postingTextResolver{q: &fakeDescriptions{}, model: m, source: "adp"}
	if got, err := r.Name(context.Background(), "b"); got != "" || err != nil || m.calls != 0 {
		t.Errorf("no descriptions: Name = %q, %v after %d calls; want empty, nil, 0 calls", got, err, m.calls)
	}

	m = &fakeModel{err: errors.New("gateway down")}
	r = postingTextResolver{q: &fakeDescriptions{rows: []string{"Acme is hiring"}}, model: m, source: "adp"}
	if _, err := r.Name(context.Background(), "b"); err == nil {
		t.Error("model error: want it returned, got nil")
	}
}

func TestCapProvider(t *testing.T) {
	rows := []db.ListPlaceholderCompanyBoardsRow{
		{Provider: "adp", Board: "a1"}, {Provider: "oracle", Board: "o1"},
		{Provider: "adp", Board: "a2"}, {Provider: "adp", Board: "a3"},
	}
	boards := func(rs []db.ListPlaceholderCompanyBoardsRow) string {
		var out []string
		for _, r := range rs {
			out = append(out, r.Board)
		}
		return strings.Join(out, ",")
	}
	for offset, want := range map[int64]string{
		0: "a1,o1,a2", // other providers always kept
		1: "o1,a2,a3",
		2: "a1,o1,a3", // the window wraps
		5: "a1,o1,a3", // offset is taken modulo the provider's rows
	} {
		if got := boards(capProvider(rows, "adp", 2, offset)); got != want {
			t.Errorf("capProvider(offset %d) = %s, want %s", offset, got, want)
		}
	}
	if len(capProvider(rows, "adp", 0, 7)) != len(rows) {
		t.Error("capProvider(n=0) should mean unbounded")
	}
}

// Successive days must reach every board: a fixed window would fill with the boards the
// model refuses and never move past them.
func TestCapProviderSweepsEveryBoard(t *testing.T) {
	var rows []db.ListPlaceholderCompanyBoardsRow
	for i := range 7 {
		rows = append(rows, db.ListPlaceholderCompanyBoardsRow{Provider: "adp", Board: string(rune('a' + i))})
	}
	seen := map[string]bool{}
	for day := int64(0); day < 4; day++ { // ceil(7/2) days
		for _, r := range capProvider(rows, "adp", 2, day*2) {
			seen[r.Board] = true
		}
	}
	if len(seen) != len(rows) {
		t.Errorf("four daily runs of 2 reached %d of %d boards", len(seen), len(rows))
	}
}
