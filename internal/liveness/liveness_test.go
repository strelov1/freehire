package liveness

import "testing"

// A realistic healthy posting body: well over the minimum length and free of any
// expired/listing signal. Contains an "Apply" control to prove that apply text on
// its own never marks a page expired (we close only on positive death signals).
const healthyBody = `Senior Go Engineer at Acme. We are hiring an experienced backend
engineer to build our job ingestion pipeline. You will work with PostgreSQL, design
HTTP APIs, and own the source adapters. Requirements: 5 years of Go, strong SQL,
distributed systems experience. Benefits include remote work and equity. Apply now
to join our team and submit your application through the form below.`

func TestClassify(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		finalURL    string
		body        string
		wantVerdict Verdict
		wantReason  string
	}{
		{
			name:        "http 404 is gone",
			status:      404,
			finalURL:    "https://boards.example.com/jobs/123",
			body:        "Not Found",
			wantVerdict: Expired,
			wantReason:  "http_gone",
		},
		{
			name:        "http 410 is gone",
			status:      410,
			finalURL:    "https://boards.example.com/jobs/123",
			body:        healthyBody,
			wantVerdict: Expired,
			wantReason:  "http_gone",
		},
		{
			name:        "redirect to error url",
			status:      200,
			finalURL:    "https://boards.example.com/jobs/123?error=true",
			body:        healthyBody,
			wantVerdict: Expired,
			wantReason:  "expired_url",
		},
		{
			name:        "closed-posting body pattern",
			status:      200,
			finalURL:    "https://boards.example.com/jobs/123",
			body:        "This role is no longer accepting applications. Browse our other openings.",
			wantVerdict: Expired,
			wantReason:  "expired_body",
		},
		{
			name:        "position filled body pattern",
			status:      200,
			finalURL:    "https://boards.example.com/jobs/123",
			body:        "We're sorry, this position has been filled. Thank you for your interest in working with us.",
			wantVerdict: Expired,
			wantReason:  "expired_body",
		},
		{
			name:        "german closed pattern",
			status:      200,
			finalURL:    "https://boards.example.com/jobs/123",
			body:        "Diese Stelle ist bereits besetzt. Schauen Sie sich unsere anderen Angebote an.",
			wantVerdict: Expired,
			wantReason:  "expired_body",
		},
		{
			// habr_career serves an archived vacancy as a healthy 200 whose only death
			// signal is the Russian "Вакансия в архиве" banner — the source's URL is the
			// posting itself, so this body is all the liveness worker has to close on.
			name:     "russian archived pattern",
			status:   200,
			finalURL: "https://career.habr.com/vacancies/1000166878",
			body: `Fullstack-Разработчик (PHP/Bitrix/Laravel) в компании Changellenge. Москва,
Санкт-Петербург. Можно удаленно. Навыки: CMS «1С-Битрикс», Laravel, PHP, Git, Vue.js.
Квалификация Middle. Полное описание вакансии, требования к кандидату, условия работы и
контакты работодателя. Вакансия в архиве. Смотрите другие открытые вакансии компании.`,
			wantVerdict: Expired,
			wantReason:  "expired_body",
		},
		{
			name:        "listing page instead of posting",
			status:      200,
			finalURL:    "https://boards.example.com/careers",
			body:        "Search our open roles. 42 jobs found across engineering, sales and marketing teams worldwide.",
			wantVerdict: Expired,
			wantReason:  "listing_page",
		},
		{
			name:        "empty shell is insufficient content",
			status:      200,
			finalURL:    "https://boards.example.com/jobs/123",
			body:        "Loading...",
			wantVerdict: Expired,
			wantReason:  "insufficient_content",
		},
		{
			name:        "healthy posting is live",
			status:      200,
			finalURL:    "https://boards.example.com/jobs/123",
			body:        healthyBody,
			wantVerdict: Live,
		},
		{
			// Transient: reached the server but got no death signal. Must be Uncertain,
			// not Live — a 503 is not evidence the posting is alive, so it must not reset
			// an accumulated strike.
			name:        "503 is uncertain, not live",
			status:      503,
			finalURL:    "https://boards.example.com/jobs/123",
			body:        "Service Unavailable",
			wantVerdict: Uncertain,
		},
		{
			name:        "403 is uncertain",
			status:      403,
			finalURL:    "https://boards.example.com/jobs/123",
			body:        "Forbidden",
			wantVerdict: Uncertain,
		},
		{
			name:        "fetch error (status 0) is uncertain",
			status:      0,
			finalURL:    "https://boards.example.com/jobs/123",
			body:        "",
			wantVerdict: Uncertain,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verdict, reason := Classify(tt.status, tt.finalURL, tt.body)
			if verdict != tt.wantVerdict {
				t.Fatalf("Classify() verdict = %v, want %v (reason %q)", verdict, tt.wantVerdict, reason)
			}
			if tt.wantVerdict == Expired && reason != tt.wantReason {
				t.Fatalf("Classify() reason = %q, want %q", reason, tt.wantReason)
			}
		})
	}
}
