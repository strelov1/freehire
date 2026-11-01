package search

import (
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestApplyQuestionsMaxFilter(t *testing.T) {
	cases := []struct {
		raw  string
		want string // "" = no bound
	}{
		{"3", "apply_questions <= 3"},
		{"0", "apply_questions <= 0"},
		{" 2 ", "apply_questions <= 2"},
		{"-1", ""},
		{"short", ""},
		{"", ""},
	}
	for _, tc := range cases {
		f := filterFromValues(url.Values{ApplyQuestionsMaxParam: {tc.raw}}, time.Now())
		got := ""
		if f != nil {
			got = fmt.Sprint(f)
		}
		if tc.want == "" {
			if strings.Contains(got, "apply_questions") {
				t.Errorf("%q: filter %q, want no apply_questions bound", tc.raw, got)
			}
			continue
		}
		if !strings.Contains(got, tc.want) {
			t.Errorf("%q: filter %s, want it to contain %q", tc.raw, f, tc.want)
		}
	}
}

// A declined bound widens the answer to every posting, so it must be named.
func TestApplyQuestionsMaxReportsDeclinedValues(t *testing.T) {
	got := IgnoredValues(url.Values{ApplyQuestionsMaxParam: {"short"}})
	if want := map[string][]string{ApplyQuestionsMaxParam: {"short"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("IgnoredValues = %v, want %v", got, want)
	}
	if got := IgnoredValues(url.Values{ApplyQuestionsMaxParam: {"-2"}}); len(got[ApplyQuestionsMaxParam]) != 1 {
		t.Errorf("IgnoredValues(-2) = %v, want it named", got)
	}
	if got := IgnoredValues(url.Values{ApplyQuestionsMaxParam: {"0"}}); got != nil {
		t.Errorf("IgnoredValues(0) = %v, want nil: zero is a valid bound", got)
	}
	if got := UnknownParams(url.Values{ApplyQuestionsMaxParam: {"3"}}, nil); len(got) != 0 {
		t.Errorf("UnknownParams = %v, want the param known", got)
	}
}

// The count rides the document only when a form was captured: absent is "unknown",
// and a present zero is a real answer that must survive serialisation.
func TestAttachApplyQuestions(t *testing.T) {
	doc, err := FromJob(db.Job{ID: 7, PublicSlug: "s", Title: "Engineer"}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	doc.AttachApplyQuestions(map[int64]int{8: 4})
	raw, _ := json.Marshal(doc)
	if strings.Contains(string(raw), `"apply_questions"`) {
		t.Errorf("document without a form carries apply_questions: %s", raw)
	}
	doc.AttachApplyQuestions(map[int64]int{7: 0})
	raw, _ = json.Marshal(doc)
	if !strings.Contains(string(raw), `"apply_questions":0`) {
		t.Errorf("document with an empty form lost its zero: %s", raw)
	}
}
