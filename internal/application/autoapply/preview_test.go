package autoapply

import "testing"

func TestResolvedPreview_AsksCandidate(t *testing.T) {
	cases := []struct {
		name    string
		pending []PreviewPending
		want    bool
	}{
		{"nothing pending", nil, false},
		{"only drafted at submission", []PreviewPending{{Label: "Why us?", WillDraftAtSubmission: true}}, false},
		{"one the candidate must answer", []PreviewPending{
			{Label: "Why us?", WillDraftAtSubmission: true},
			{Label: "Desired salary"},
		}, true},
	}
	for _, c := range cases {
		if got := (ResolvedPreview{Pending: c.pending}).AsksCandidate(); got != c.want {
			t.Errorf("%s: AsksCandidate() = %v, want %v", c.name, got, c.want)
		}
	}
}
