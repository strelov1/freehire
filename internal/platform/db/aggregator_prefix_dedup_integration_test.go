//go:build integration

package db

import (
	"context"
	"strings"
	"testing"
)

func TestSuppressAggregator_HimalayasMidWordPrefix(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	ats := atsJob("484025180", "Applied AI Engineer – OCR, Dokumentenklassifikation, Extraktion, RAG/LLM, Evaluation/QA (m/w/d) – Remote – Freelancer", []string{"DE"})
	ats.Source = "personio"
	mustUpsert(t, q, ats)
	copies := []string{"1626396644", "580089259", "365294833", "218244261"}
	for _, id := range copies {
		mustUpsert(t, q, aggJob(id, "Applied AI Engineer – OCR, Dokumentenklassifikation, Extraktion, RAG/LLM, Evalua", []string{"DE"}))
	}

	suppressAggregators(t, q)
	atsID, atsDup := dupOf(t, pool, ats.ExternalID)
	if atsDup != -1 {
		t.Errorf("ATS duplicate_of = %d, want NULL", atsDup)
	}
	for _, id := range copies {
		if _, dup := dupOf(t, pool, id); dup != atsID {
			t.Errorf("copy %s duplicate_of = %d, want ATS %d", id, dup, atsID)
		}
	}

	ctx := context.Background()
	var deleted int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM search_delete_outbox").Scan(&deleted); err != nil {
		t.Fatal(err)
	}
	if deleted != len(copies) {
		t.Errorf("search deletes = %d, want %d", deleted, len(copies))
	}
	changed, err := q.SuppressAggregatorDuplicatesForCompanies(ctx, SuppressAggregatorDuplicatesForCompaniesParams{
		FoldedCompanies: []string{"acme"}, Aggregators: aggregators,
	})
	if err != nil || changed != 0 {
		t.Fatalf("rerun changed = %d, err = %v, want 0 changes", changed, err)
	}
	if _, err := pool.Exec(ctx, "TRUNCATE search_outbox"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE jobs SET closed_at = now() WHERE id = $1", atsID); err != nil {
		t.Fatal(err)
	}
	suppressAggregators(t, q)
	for _, id := range copies {
		jobID, dup := dupOf(t, pool, id)
		if dup != -1 {
			t.Errorf("released copy %s duplicate_of = %d, want NULL", id, dup)
		}
		var queued bool
		if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM search_outbox WHERE job_id = $1)", jobID).Scan(&queued); err != nil {
			t.Fatal(err)
		}
		if !queued {
			t.Errorf("released copy %s missing from search outbox", id)
		}
	}
}

func TestSuppressAggregator_HimalayasPrefixGuards(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	const full = "Distributed Systems Engineer for Global Payment Processing Platform Infrastructure Services"
	const prefix = "Distributed Systems Engineer for Global Payment Processing Platform Infrastructu"
	const fourTokens = "Distributed Infrastructure Observability Engin"
	for _, tc := range []struct {
		name       string
		atsTitle   string
		aggTitle   string
		atsCountry []string
		aggCountry []string
		company    string
		source     string
		state      string
		match      bool
	}{
		{name: "normalized and folded company", atsTitle: full, aggTitle: strings.Replace(strings.ToUpper(prefix), " ", "_", 1), company: "ac-me", match: true},
		{name: "empty ATS country", atsTitle: full, aggTitle: prefix, aggCountry: []string{"US"}, match: true},
		{name: "empty aggregator country", atsTitle: full, aggTitle: prefix, atsCountry: []string{"US"}, match: true},
		{name: "different company", atsTitle: full, aggTitle: prefix, company: "other"},
		{name: "different country", atsTitle: full, aggTitle: prefix, atsCountry: []string{"US"}, aggCountry: []string{"DE"}},
		{name: "complete final word", atsTitle: "Senior Software Platform Infrastructure Engineering Manager", aggTitle: "Senior Software Platform Infrastructure Engineer"},
		{name: "below feed ceiling", atsTitle: full, aggTitle: prefix[:79]},
		{name: "above feed ceiling", atsTitle: full, aggTitle: full[:81]},
		{name: "short title", atsTitle: "Applied AI Engineer Evaluation", aggTitle: "Applied AI Engineer Evalua"},
		{name: "empty title", atsTitle: full},
		{name: "short fragment", atsTitle: full, aggTitle: strings.TrimSuffix(prefix, "Infrastructu") + strings.Repeat(" ", 9) + "Inf"},
		{name: "too few complete tokens", atsTitle: "Distributed Infrastructure Observability Engineering", aggTitle: strings.Replace(fourTokens, " ", strings.Repeat(" ", 81-len(fourTokens)), 1)},
		{name: "non prefix", atsTitle: full, aggTitle: strings.Replace(prefix, "Payment", "Payroll", 1)},
		{name: "literal punctuation", atsTitle: full, aggTitle: strings.Replace(prefix, "Payment", "Pay%ent", 1)},
		{name: "reversed", atsTitle: prefix[:79], aggTitle: prefix},
		{name: "other aggregator", atsTitle: full, aggTitle: prefix, source: "gulftalent"},
		{name: "private exact ATS", atsTitle: full, aggTitle: full, state: "private"},
		{name: "private suffix ATS", atsTitle: full, aggTitle: full + " - Remote", state: "private"},
		{name: "private subset ATS", atsTitle: full + " Platform", aggTitle: full, state: "private"},
		{name: "private ATS", atsTitle: full, aggTitle: prefix, state: "private"},
		{name: "closed ATS", atsTitle: full, aggTitle: prefix, state: "closed"},
		{name: "duplicate ATS", atsTitle: full, aggTitle: prefix, state: "duplicate"},
		{name: "role owned aggregator", atsTitle: full, aggTitle: prefix, state: "role"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t, pool)
			ats := atsJob("acme:ats", tc.atsTitle, tc.atsCountry)
			agg := aggJob("acme:agg", tc.aggTitle, tc.aggCountry)
			if tc.company != "" {
				agg.CompanySlug = tc.company
			}
			if tc.source != "" {
				agg.Source = tc.source
			}
			mustUpsert(t, q, ats)
			mustUpsert(t, q, agg)
			if tc.company == "other" {
				// Put both companies in the batch so the join itself must keep them apart.
				mustUpsert(t, q, aggJob("acme:driver", "Unrelated role", nil))
			}
			atsID, _ := dupOf(t, pool, ats.ExternalID)
			aggID, _ := dupOf(t, pool, agg.ExternalID)
			want := int64(-1)
			ctx := context.Background()
			switch tc.state {
			case "private":
				if _, err := pool.Exec(ctx, "UPDATE jobs SET is_private = true, source = 'pasted' WHERE id = $1", atsID); err != nil {
					t.Fatal(err)
				}
			case "closed":
				if _, err := pool.Exec(ctx, "UPDATE jobs SET closed_at = now() WHERE id = $1", atsID); err != nil {
					t.Fatal(err)
				}
			case "duplicate", "role":
				mustUpsert(t, q, aggJob("acme:owner", "Unrelated role", nil))
				ownerID, _ := dupOf(t, pool, "acme:owner")
				id := atsID
				if tc.state == "role" {
					id, want = aggID, ownerID
				}
				if _, err := pool.Exec(ctx, "UPDATE jobs SET duplicate_of_role = $1 WHERE id = $2", ownerID, id); err != nil {
					t.Fatal(err)
				}
			}
			if tc.match {
				want = atsID
			}
			suppressAggregators(t, q)
			if _, dup := dupOf(t, pool, agg.ExternalID); dup != want {
				t.Errorf("aggregator duplicate_of = %d, want %d", dup, want)
			}
		})
	}
}
