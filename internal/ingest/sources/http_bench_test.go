package sources

import (
	"strings"
	"testing"
)

// benchListing is the shape of a JSON job board's listing endpoint: a page of jobs with
// nested objects, an array, numbers, a boolean, a null and a field the struct does not name.
var benchListing = `{"page":1,"total":137,"jobs":[` +
	benchJobs(20) +
	`]}`

func benchJobs(n int) string {
	job := `{"id":"jb-48213","title":"Senior Backend Engineer (Go, Postgres)","department":{"name":"Platform","code":"R&D"},"location":{"city":"Berlin","country":"Germany","remote":true},"tags":["go","postgresql","kubernetes"],"salary":95000.50,"applyUrl":"https://ats.example/jobs/jb-48213","featured":false,"closedAt":null,"legacyRef":"x-991"},`
	return strings.TrimSuffix(strings.Repeat(job, n), ",")
}

type benchJob struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Department struct {
		Name string `json:"name"`
		Code string `json:"code"`
	} `json:"department"`
	Location struct {
		City    string `json:"city"`
		Country string `json:"country"`
		Remote  bool   `json:"remote"`
	} `json:"location"`
	Tags     []string `json:"tags"`
	Salary   float64  `json:"salary"`
	ApplyURL string   `json:"applyUrl"`
	Featured bool     `json:"featured"`
	ClosedAt *string  `json:"closedAt"`
}

type benchListingDoc struct {
	Page  int        `json:"page"`
	Total int        `json:"total"`
	Jobs  []benchJob `json:"jobs"`
}

// BenchmarkDecodeJSON times the one decoder every JSON getter shares (decodeJSON), on a
// listing-sized body read from memory.
func BenchmarkDecodeJSON(b *testing.B) {
	var want benchListingDoc
	if err := decodeJSON(strings.NewReader(benchListing), &want); err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(benchListing)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var doc benchListingDoc
		if err := decodeJSON(strings.NewReader(benchListing), &doc); err != nil {
			b.Fatal(err)
		}
	}
}
