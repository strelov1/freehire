package telegram

import (
	"strconv"
	"testing"
)

const (
	eaURL       = "https://jobs.ea.com/en_US/careers/JobDetail/Software-Engineer-Intern/215949"
	whatsappURL = "https://whatsapp.com/channel/0029Va9sXNw3AzNa6mBs060w"
)

// bare is a link whose text is its own address, the way Telegram renders a URL typed into
// a post.
func bare(u string) Link { return Link{Text: u, URL: u} }

func TestApplyURL(t *testing.T) {
	linkedin := Link{Text: "Apply Here", URL: "https://www.linkedin.com/jobs/view/4482431822/"}
	tests := []struct {
		name  string
		links []Link
		text  string
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

		// A bare address the post LABELS as the way to apply.
		{
			// Report #156, t.me/goyalarsh/6410: the vacancy's URL behind "Link:", the channel's
			// WhatsApp footer behind words that are not a label.
			name: "bare address behind a Link label, beside a footer",
			text: "Company: Electronic Arts\nRole: SDE Internship\nBatch: 2027/28 passouts\n\n" +
				"Link: " + eaURL + "\n\nJoin " + whatsappURL + " for some cool opportunities.",
			links: []Link{bare(eaURL), bare(whatsappURL)},
			jobs:  1,
			want:  eaURL,
		},
		{name: "apply label on the same line", jobs: 1,
			text:  "Fleet Manager\n🔗 Apply here: http://jobs.example.ng/job/fleet-manager/",
			links: []Link{bare("http://jobs.example.ng/job/fleet-manager/")},
			want:  "http://jobs.example.ng/job/fleet-manager/"},
		{
			name: "apply label on the line above",
			text: "Graduate Trainee\n\nSee full details and apply: \nhttps://jobs.example.com/trainee/\n\n" +
				"Follow us on Telegram via:\nhttps://t.me/cv_2essence",
			links: []Link{bare("https://jobs.example.com/trainee/"), bare("https://t.me/cv_2essence")},
			jobs:  1,
			want:  "https://jobs.example.com/trainee/",
		},
		{name: "a label is read by its last words", jobs: 1,
			text:  "Junior QA\nFollow us on Twitter: https://x.com/acme\nApply link: https://acme.example.com/jobs/7",
			links: []Link{bare("https://x.com/acme"), bare("https://acme.example.com/jobs/7")},
			want:  "https://acme.example.com/jobs/7"},
		{name: "a bare Link label must stand alone", jobs: 1,
			text:  "Junior QA\nWhatsApp community link: https://chat.whatsapp.com/abc\nhttps://acme.example.com/jobs/7",
			links: []Link{bare("https://chat.whatsapp.com/abc"), bare("https://acme.example.com/jobs/7")}},

		// The post's only outbound link, standing on a line of its own.
		{
			// t.me/STEMJobsCR: a title, three facts and the URL, nothing else.
			name: "sole address on its own line",
			text: "🧑‍💼  |  Product Support Engineer\n\nEmpresa: Equifax\nUbicación: Pune\nTags: #soporte\n\n" +
				"https://careers.equifax.com/en/jobs/j00178866/product-support-engineer/",
			links: []Link{
				{Text: "#soporte", URL: "?q=%23soporte"},
				bare("https://careers.equifax.com/en/jobs/j00178866/product-support-engineer/"),
			},
			jobs: 1,
			want: "https://careers.equifax.com/en/jobs/j00178866/product-support-engineer/",
		},
		{name: "sole address decorated only by an emoji", jobs: 1,
			text:  "Intern JavaScript Developer\n👉 https://dou.ua/calendar/58283/?from=tg",
			links: []Link{bare("https://dou.ua/calendar/58283/?from=tg")},
			want:  "https://dou.ua/calendar/58283/?from=tg"},
		{name: "sole address beside words is a footer", jobs: 1,
			text:  "Backend Engineer, DM @hr\nJoin " + whatsappURL + " for some cool opportunities.",
			links: []Link{bare(whatsappURL)}},
		{name: "sole address naming a site, not a page", jobs: 1,
			text:  "Backend Engineer at Acme\nhttps://acme.example.com/",
			links: []Link{bare("https://acme.example.com/")}},
		{name: "two unlabelled addresses are ambiguous", jobs: 1,
			text:  "Backend Engineer\nhttps://acme.example.com/jobs/1\nhttps://acme.example.com/blog/2",
			links: []Link{bare("https://acme.example.com/jobs/1"), bare("https://acme.example.com/blog/2")}},
		{name: "a worded link is never taken for a sole address", jobs: 1,
			text:  "Backend Engineer\nРазместить вакансию",
			links: []Link{{Text: "Разместить вакансию", URL: "https://www.vseti.app/newcv"}}},
		{name: "a labelled link still needs a single-vacancy post", jobs: 2,
			text:  "Two roles\nLink: " + eaURL,
			links: []Link{bare(eaURL)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := applyURL(tc.text, tc.links, tc.jobs); got != tc.want {
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

// Report #156: the label that names the apply link lives in the post's TEXT, so draftJobs
// must hand the text over, not only the links.
func TestDraftJobsReadsTheLabelInThePostText(t *testing.T) {
	post := pendingPost()
	post.Text = "Company: Electronic Arts\nLink: " + eaURL + "\nJoin " + whatsappURL + " for more."
	post.Links = []Link{bare(eaURL), bare(whatsappURL)}
	built, _ := ExtractRunner{}.draftJobs(post, []ExtractedJob{
		{Title: "Software Engineer Intern", Company: "Electronic Arts", Description: "C++."},
	})
	if len(built) != 1 || built[0].Fields().URL != eaURL {
		t.Fatalf("built %+v, want one job at %q", built, eaURL)
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
