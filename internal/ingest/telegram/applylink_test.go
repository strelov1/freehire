package telegram

import (
	"strconv"
	"testing"
)

func TestApplyURL(t *testing.T) {
	linkedin := Link{Text: "Apply Here", URL: "https://www.linkedin.com/jobs/view/4482431822/"}
	tests := []struct {
		name  string
		links []Link
		jobs  int
		want  string
	}{
		{
			// The reported post, t.me/cs_algo/22398: the apply link sits among hashtag links
			// and a Telegram folder invite.
			name: "apply link among hashtags and a telegram invite",
			links: []Link{
				linkedin,
				{Text: "#cs_algo", URL: "?q=%23cs_algo"},
				{Text: "#hiring", URL: "https://t.me/s/cs_algo?q=%23hiring"},
				{Text: "Join our folder", URL: "https://t.me/addlist/abcDEF"},
			},
			jobs: 1,
			want: linkedin.URL,
		},
		{name: "russian apply verb", jobs: 1,
			links: []Link{{Text: "Откликнуться", URL: "https://youngjunior.ru/vacancy/123"}},
			want:  "https://youngjunior.ru/vacancy/123"},
		{name: "arrowed apply phrase", jobs: 1,
			links: []Link{{Text: "→ Откликнуться на вакансию", URL: "https://hh.ru/vacancy/1"}},
			want:  "https://hh.ru/vacancy/1"},
		{name: "by the link", jobs: 1,
			links: []Link{{Text: "ссылке", URL: "https://facancy.ru/v/9"}},
			want:  "https://facancy.ru/v/9"},
		{name: "here in russian", jobs: 1,
			links: []Link{{Text: "тут", URL: "https://careers.example.com/42"}},
			want:  "https://careers.example.com/42"},
		{name: "vacancy description", jobs: 1,
			links: []Link{{Text: "Описание вакансии.", URL: "https://acme.huntflow.io/vacancy/7"}},
			want:  "https://acme.huntflow.io/vacancy/7"},
		{name: "first apply link wins", jobs: 1,
			links: []Link{
				{Text: "Apply", URL: "https://jobs.smartrecruiters.com/Acme/1"},
				{Text: "apply here", URL: "https://jobs.smartrecruiters.com/Acme/2"},
			},
			want: "https://jobs.smartrecruiters.com/Acme/1"},
		{name: "apply link after an ad link", jobs: 1,
			links: []Link{
				{Text: "Разместить вакансию", URL: "https://www.vseti.app/newcv"},
				{Text: "Apply", URL: "https://acme.example.com/jobs/1"},
			},
			want: "https://acme.example.com/jobs/1"},

		// Refusals: every one of these keeps the post as the job's URL.
		{name: "no links", jobs: 1, want: ""},
		{name: "bare-url text is a channel footer", jobs: 1,
			links: []Link{{Text: "https://wantapply.com", URL: "https://wantapply.com"}}},
		{name: "schemeless bare-url text whose host says apply", jobs: 1,
			links: []Link{{Text: "wantapply.com", URL: "https://wantapply.com/"}}},
		{name: "truncated bare-url text", jobs: 1,
			links: []Link{{Text: "https://www.linkedin.com/jobs/view/44…", URL: "https://www.linkedin.com/jobs/view/4471432326/"}}},
		{name: "telegram host even with apply text", jobs: 1,
			links: []Link{{Text: "Apply", URL: "https://t.me/hr_bot"}}},
		{name: "telegram.me host", jobs: 1,
			links: []Link{{Text: "откликнуться", URL: "https://telegram.me/hr"}}},
		{name: "relative link", jobs: 1,
			links: []Link{{Text: "apply", URL: "/jobs/1"}}},
		{name: "non-http scheme", jobs: 1,
			links: []Link{{Text: "apply", URL: "mailto:hr@acme.com"}}},
		{name: "hashtag text", jobs: 1,
			links: []Link{{Text: "#apply", URL: "https://example.com/?q=%23apply"}}},
		{name: "hashtag query", jobs: 1,
			links: []Link{{Text: "apply", URL: "https://example.com/s/chan?q=#apply"}}},
		{name: "text with no apply word is not guessed at", jobs: 1,
			links: []Link{{Text: "Acme Corp", URL: "https://acme.example.com"}}},
		{name: "apply word must be a whole word", jobs: 1,
			links: []Link{{Text: "Where we are", URL: "https://acme.example.com/about"}}},
		{name: "multi-job post keeps the post", jobs: 2,
			links: []Link{linkedin}},
		{name: "no jobs", jobs: 0, links: []Link{linkedin}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := applyURL(tc.links, tc.jobs); got != tc.want {
				t.Errorf("applyURL = %q, want %q", got, tc.want)
			}
		})
	}
}

// The bug report: a single-job post that carries its own apply link must point there, not at
// the Telegram post — while the identity stays the post's, so dedup is unaffected.
func TestDraftJobsUsesThePostsApplyLink(t *testing.T) {
	post := pendingPost()
	post.Links = []Link{
		{Text: "Apply Here", URL: "https://www.linkedin.com/jobs/view/4482431822/"},
		{Text: "Join", URL: "https://t.me/addlist/abc"},
	}
	built, skipped := ExtractRunner{}.draftJobs(post, []ExtractedJob{
		{Title: "Software Engineer", Company: "Acme", Description: "Go."},
	})
	if skipped != 0 || len(built) != 1 {
		t.Fatalf("built %d, skipped %d; want 1/0", len(built), skipped)
	}
	base := post.Channel + "/" + strconv.FormatInt(post.MsgID, 10)
	got := built[0].Fields()
	if got.URL != "https://www.linkedin.com/jobs/view/4482431822/" {
		t.Errorf("URL = %q, want the post's apply link", got.URL)
	}
	if got.ExternalID != base+"/0" {
		t.Errorf("ExternalID = %q, want %q", got.ExternalID, base+"/0")
	}
}

// A post naming several roles cannot say which link belongs to which, so every job keeps the
// post itself.
func TestDraftJobsMultiJobPostKeepsThePost(t *testing.T) {
	post := pendingPost()
	post.Links = []Link{{Text: "Apply", URL: "https://acme.example.com/jobs"}}
	built, _ := ExtractRunner{}.draftJobs(post, []ExtractedJob{
		{Title: "Backend Engineer", Company: "Acme", Description: "Go."},
		{Title: "Frontend Engineer", Company: "Acme", Description: "TS."},
	})
	want := "https://t.me/" + post.Channel + "/" + strconv.FormatInt(post.MsgID, 10)
	for _, j := range built {
		if j.Fields().URL != want {
			t.Errorf("URL = %q, want the post %q", j.Fields().URL, want)
		}
	}
}
