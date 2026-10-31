package qualityaudit

import (
	"context"
	"errors"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/platform/llm"
)

type fakeModel struct {
	answer       string
	err          error
	system, user string
}

func (f *fakeModel) GenerateJSON(_ context.Context, system, user string, _ ...llm.GenOption) (string, error) {
	f.system, f.user = system, user
	return f.answer, f.err
}

var sample = RemotePosting{
	ID:          7,
	Title:       "Backend Engineer",
	Location:    "Remote - Europe",
	Description: "<p>We are a <b>fully   remote</b> team.</p><p>You must live in the EU.</p>",
}

func TestJudgeRemoteGrounded(t *testing.T) {
	m := &fakeModel{answer: "```json\n" + `{"work_mode":"remote","work_mode_quote":"Fully remote team","worldwide":"no","worldwide_quote":"you must live in the EU"}` + "\n```"}
	got, err := JudgeRemote(context.Background(), m, sample)
	if err != nil {
		t.Fatal(err)
	}
	if got != (Judgement{WorkMode: ModeRemote, Worldwide: WorldwideNo}) {
		t.Errorf("judgement = %+v", got)
	}
	// The posting reaches the model as text, and our own tags never do: a judge shown "remote"
	// would anchor on it.
	for _, want := range []string{"Backend Engineer", "Remote - Europe", "fully remote", "You must live in the EU."} {
		if !strings.Contains(m.user, want) {
			t.Errorf("prompt lacks %q:\n%s", want, m.user)
		}
	}
	for _, leak := range []string{"<p>", "global", "work_mode"} {
		if strings.Contains(m.user, leak) {
			t.Errorf("prompt carries %q:\n%s", leak, m.user)
		}
	}
	if !strings.Contains(m.system, "unclear") {
		t.Error("the system prompt must offer unclear")
	}
}

func TestJudgeRemoteDemotesWhatItCannotGround(t *testing.T) {
	cases := map[string]string{
		"quote not in the posting": `{"work_mode":"remote","work_mode_quote":"work from anywhere","worldwide":"yes","worldwide_quote":"anywhere in the world"}`,
		"no quote":                 `{"work_mode":"onsite","work_mode_quote":"","worldwide":"no","worldwide_quote":""}`,
		"outside the vocabulary":   `{"work_mode":"flexible","work_mode_quote":"fully remote","worldwide":"maybe","worldwide_quote":"EU"}`,
		"not JSON":                 `I think it is remote`,
	}
	for name, answer := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := JudgeRemote(context.Background(), &fakeModel{answer: answer}, sample)
			if err != nil {
				t.Fatal(err)
			}
			if got != (Judgement{WorkMode: ModeUnclear, Worldwide: WorldwideUnclear}) {
				t.Errorf("judgement = %+v, want unclear on both", got)
			}
		})
	}
}

func TestJudgeRemoteUnclearNeedsNoQuote(t *testing.T) {
	got, err := JudgeRemote(context.Background(), &fakeModel{answer: `{"work_mode":"unclear","work_mode_quote":"","worldwide":"unclear","worldwide_quote":""}`}, sample)
	if err != nil || got != (Judgement{WorkMode: ModeUnclear, Worldwide: WorldwideUnclear}) {
		t.Errorf("judgement = %+v, %v", got, err)
	}
}

func TestJudgeRemoteModelError(t *testing.T) {
	if _, err := JudgeRemote(context.Background(), &fakeModel{err: errors.New("gateway down")}, sample); err == nil {
		t.Error("a model failure must be an error, not an unclear judgement")
	}
}

func TestChooseSources(t *testing.T) {
	weights := []SourceWeight{{"a", 1000}, {"b", 10}, {"c", 0}, {"d", 500}}
	rng := rand.New(rand.NewPCG(3, 4))
	got := names(ChooseSources(weights, 2, rng))
	if len(got) != 2 || got[0] == got[1] || slices.Contains(got, "c") {
		t.Errorf("ChooseSources = %v", got)
	}
	all := names(ChooseSources(weights, 10, rng))
	slices.Sort(all)
	if !slices.Equal(all, []string{"a", "b", "d"}) {
		t.Errorf("k above the eligible count = %v, want every weighted source once", all)
	}

	// The heaviest source is drawn far more often than the lightest.
	picks := map[string]int{}
	for i := range 2000 {
		for _, s := range ChooseSources(weights, 1, rand.New(rand.NewPCG(uint64(i), 9))) {
			picks[s.Source]++
		}
	}
	if picks["a"] < 5*picks["b"] || picks["b"] == 0 {
		t.Errorf("draws = %v, want a far above b and b drawn at all", picks)
	}
}

func names(ws []SourceWeight) []string {
	out := make([]string, 0, len(ws))
	for _, w := range ws {
		out = append(out, w.Source)
	}
	return out
}
