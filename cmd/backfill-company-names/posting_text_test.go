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
	got := capProvider(rows, "adp", 2)
	var boards []string
	for _, r := range got {
		boards = append(boards, r.Board)
	}
	if strings.Join(boards, ",") != "a1,o1,a2" {
		t.Errorf("capProvider = %v, want a1,o1,a2 (other providers untouched)", boards)
	}
	if len(capProvider(rows, "adp", 0)) != len(rows) {
		t.Error("capProvider(0) should mean unbounded")
	}
}
