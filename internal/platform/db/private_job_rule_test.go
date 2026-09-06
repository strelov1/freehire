package db

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// privatePredicateExemptions names every statement that reads the open catalogue and may
// legitimately see a private posting, with the reason it may.
//
// "Legitimately" means one thing here: the private posting cannot reach anybody but its
// creator, and cannot change an answer given to anybody else. Cost is a different
// question and deliberately not this rule's — a private row that draws an LLM call is
// wrong for another reason, and exempting it here says nothing about that.
//
// Three anti-tautology checks keep the list from becoming "everything that fails today":
// a name matching no statement fails, a name outside the population fails, and a name
// that already carries the predicate fails. An entry can only survive by describing a
// statement that really does read open postings without the clause.
var privatePredicateExemptions = map[string]string{
	// --- Owner-scoped: the only reader is the posting's own creator. ---
	"ListUserApplicationsForMatch": "the caller's own tracked applications (user_jobs.user_id = $1); a private posting here is one they created",

	// --- Cannot reach a private row at all: the source or the queue never holds one.
	// A private posting's source is 'pasted' or 'weblink' (internal/job/privatejob), and
	// nothing crawls either. ---
	"ListAggregatorJobsForCrosscheckBySource": "scoped to one aggregator source, which a private posting's source is never one of",
	"ListCompanyBoardTitles":                  "scoped to the ats/company board sources, which a private posting's source is never one of",
	"UnseenJobIDs":                            "the post-ingest close sweep, scoped to the source a crawl just ran and the company slugs it touched",
	"UnseenJobIDsBySource":                    "the same sweep, scoped to one source; a private posting's source is never crawled",

	// --- Writes back to the private row itself: the effect stays inside the row, so it
	// reaches its creator and nobody else. ---
	"EnqueuePendingJobs":               "queues a row for enrichment; the model's answer is written back to that same row",
	"EnqueueEnrichmentForCompanySlugs": "the same queue, scoped to named company slugs",
	"ClaimEnrichmentBatch":             "leases enrichment entries; the jobs join only orders them by freshness and skips closed rows",
	"EnqueuePendingSemanticJobs":       "queues embedding for a row; the one reader of those vectors that answers anyone else, NearestJobsToJob, excludes private candidates itself",
	"SelectOrphanLivenessCandidates":   "the liveness probe's candidate set; a strike or a close changes what the creator sees, not what anyone else does",
	"SelectStaleRegisteredCandidates":  "the same probe's registered-source candidate set",
	"MarkLivenessExpired":              "the strike write for one probed row",
	"ListJobsForRequirementsBackfill":  "a one-off backfill reading a description to fill that row's own requirements_derived",
	"ResidualTitleGroups": "cmd/mine-titles's operator report: 2-3 word groups mined from live " +
		"unclassified titles, read by a person and copied into the non-tech dictionary. The " +
		"predicate is owed here and is a follow-up, not an exemption on the merits — a pasted " +
		"JD's title fragments reaching that console is small but real. Recorded honestly rather " +
		"than blessed: the earlier reason named cmd/prune, which does not call this at all.",

	// --- Aggregates: a sample, never a posting. ---
	// The other seven insights rollups count with `count(*) FILTER (WHERE closed_at IS
	// NULL)` over the whole catalogue and so are outside this rule's shape. Exempting
	// these two keeps the family consistent rather than splitting it on which member the
	// detector happens to see.
	"RebuildInsightsSalaryStatsGlobal":    "a salary percentile over a band of at least @min_sample postings; a private one contributes a number, never a title, slug or link",
	"RebuildInsightsSalaryStatsByCountry": "the same rollup, per country",

	// --- Duplicate-marker passes: they write a pointer between two postings of one
	// company and show nothing.
	//
	// A private row that lands in a cluster is already invisible, so being marked a
	// duplicate costs it nothing. The residual is the reverse — a private row elected
	// CANON would take the public postings of that role out of search with it — and it is
	// bounded by how the canon is chosen: MIN(id), so the private row would have to
	// predate every open public posting of the role, when a pasted JD is by construction
	// copied from one that already exists. Narrowing the passes themselves means editing
	// statements that run over the whole catalogue, which is not this change. ---
	"CompaniesWithRoleClusters":                "names the companies whose role clusters the pass recomputes",
	"RecomputeRoleDuplicatesForCompanies":      "recomputes duplicate_of_role within one company's clusters",
	"CompaniesWithAggregatorPostings":          "names the companies holding aggregator postings for the suppression pass",
	"SuppressAggregatorDuplicatesForCompanies": "marks an aggregator posting a duplicate of the employer's own board row",
	"CompaniesWithFuzzyDedupCandidates":        "names the companies the fuzzy pass considers",
	"FuzzyDedupCandidateTitlesForCompany":      "the titles the fuzzy pass compares within one company",
	"MarkFuzzyDuplicatesForCompany":            "writes duplicate_of_fuzzy for one company's candidates",
	"DuplicateClosureGeoAll":                   "unions a closure's geography into its owner's facets — country/region/city codes, no posting",
	"DuplicateClosureGeoFor":                   "the same union, for named owners",

	// --- Answers only about content the caller already holds. ---
	"FindOpenJobByURL": "matches on the URL the caller supplied, and a private posting carries a URL only when it was created BY scraping that page (a pasted-text one has none), so the slug it can return always points at content the caller had the address for",
}

// privatePredicatePinned names statements that carry NOT is_private for a load-bearing
// reason but whose shape leaves them outside the population below — so the rule would not
// notice the clause being deleted again.
var privatePredicatePinned = map[string]string{
	"NearestJobsToJob": "GET /jobs/:slug/similar is unauthenticated and lists whole postings; the predicate sits on a JOIN condition, which the population detector deliberately does not read as a filter",
}

// TestEveryOpenCatalogueReadExcludesPrivateJobs pins a rule that only a comment would
// otherwise hold, and that four separate statements have now broken.
//
// A private posting is the jd-tailor-intake path: a job description ONE user pasted in,
// visible only to them, kept out of reach by a public_slug nobody can guess rather than by
// any authorization check (internal/job/privatejob, internal/ingest/jdresolve). Nothing
// about the row says so — it is open, it carries derived facets and a company_slug like
// any other posting, and jobs.is_private is the only thing that distinguishes it. So every
// statement that reads the live catalogue has to remember one clause, and four of them did
// not: /jobs/:slug/similar listed private postings as neighbours, ListJobsByCompany put
// them at the top of a company's public page, and the moderator edit path both read and
// rewrote them. Each was fixed on its own, and the next one broke anyway.
//
// The population is "reads jobs, restricted to open postings" — a statement asking what
// the catalogue currently offers. Statements keyed to one job (GetJobBySlug) or to one
// user are outside it by construction: they answer about a row somebody named, and
// jdresolve records that ownership is the caller's to check there.
func TestEveryOpenCatalogueReadExcludesPrivateJobs(t *testing.T) {
	queries := openCatalogueReads(t)
	seen := make(map[string]bool, len(queries))
	for _, q := range queries {
		seen[q.name] = true
		reason, exempt := privatePredicateExemptions[q.name]
		switch {
		case q.hasPrivatePredicate && exempt:
			t.Errorf("%s: %s carries NOT is_private and is also exempted (%q) — drop the exemption, "+
				"the statement no longer needs one", q.file, q.name, reason)
		case q.hasPrivatePredicate, exempt:
		default:
			t.Errorf("%s: %s reads open postings without NOT is_private. A private posting is one "+
				"user's pasted JD (internal/job/privatejob) and must not appear in, or change, an "+
				"answer given to anyone else. Add the clause, or name the statement in "+
				"privatePredicateExemptions with the reason it may see one.", q.file, q.name)
		}
	}

	// An exemption for a statement that is not in the population exempts nothing, and
	// would go on exempting whatever statement is next given that name.
	for name, reason := range privatePredicateExemptions {
		if !seen[name] {
			t.Errorf("privatePredicateExemptions[%q] (%q) matches no statement that reads open "+
				"postings — it was renamed, deleted, or the detector no longer sees it", name, reason)
		}
	}
	for name, reason := range privatePredicatePinned {
		if seen[name] {
			t.Errorf("privatePredicatePinned[%q] (%q) is now inside the population, which checks it "+
				"already — move it out of the pinned list", name, reason)
		}
	}

	// Counting the population, not just the violations: a detector that silently stops
	// matching looks exactly like a rule that passes. The number moves whenever a
	// statement is added or removed, which is the point — it is read, not maintained
	// blind.
	if len(queries) < 35 {
		t.Errorf("only %d statements matched as open-catalogue reads of jobs, expected at least 35 "+
			"— scopesToOpenPostings has probably drifted from how the queries are written", len(queries))
	}
}

// TestPinnedPrivatePredicatesSurvive holds the clause on the statements the population
// cannot see. Today that is the /similar rollup, the first of the four leaks and the one
// whose fix has no other guard.
func TestPinnedPrivatePredicatesSurvive(t *testing.T) {
	found := make(map[string]bool, len(privatePredicatePinned))
	for _, q := range allNamedJobsQueries(t) {
		if _, want := privatePredicatePinned[q.name]; !want {
			continue
		}
		found[q.name] = true
		if !q.hasPrivatePredicate {
			t.Errorf("%s: %s lost its NOT is_private — %s", q.file, q.name, privatePredicatePinned[q.name])
		}
	}
	for name := range privatePredicatePinned {
		if !found[name] {
			t.Errorf("privatePredicatePinned[%q] matches no statement reading jobs", name)
		}
	}
}

// TestScopesToOpenPostings pins the detector against the four shapes that mean something
// other than "restrict this read to open postings", each taken from a live statement.
func TestScopesToOpenPostings(t *testing.T) {
	for _, tc := range []struct {
		name string
		sql  string
		want bool
	}{
		{"plain where clause", "select *\nfrom jobs\nwhere closed_at is null and duplicate_of is null", true},
		{"qualified and continued", "from jobs j\nwhere j.company_slug = $1\n  and j.closed_at is null", true},
		{"aggregate filter", "select count(*) filter (where closed_at is null)::int\nfrom jobs", false},
		{"order-by preference", "from jobs j\nwhere normalize_job_url(j.url) = $1\norder by (j.closed_at is null) desc", false},
		{"join condition on a second copy", "from jobs j\nleft join jobs c on c.id = j.duplicate_of and c.closed_at is null", false},
		{"projection of the state", "select j.id,\n       (j.closed_at is null)::bool as job_open\nfrom application_nudges n\njoin jobs j on j.id = n.job_id", false},
		{"no mention at all", "select * from jobs where public_slug = $1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := scopesToOpenPostings(tc.sql); got != tc.want {
				t.Errorf("scopesToOpenPostings(%q) = %v, want %v", tc.sql, got, tc.want)
			}
		})
	}
}

type jobsQuery struct {
	file                string
	name                string
	scopedToOpen        bool
	hasPrivatePredicate bool
}

var (
	reReadsJobs   = regexp.MustCompile(`\b(from|join)\s+jobs\b`)
	reOpenScope   = regexp.MustCompile(`\bclosed_at\s+is\s+null\b`)
	rePrivatePred = regexp.MustCompile(`\bnot\s+(\w+\.)?is_private\b`)
)

// allNamedJobsQueries returns every named statement in queries/*.sql that reads the jobs
// table, judged on the SQL and not the prose around it — a comment MENTIONING is_private
// must not satisfy the rule, which is the same trap the folded-column walker beside this
// one documents.
func allNamedJobsQueries(t *testing.T) []jobsQuery {
	t.Helper()
	const queryDir = "queries"
	entries, err := os.ReadDir(queryDir)
	if err != nil {
		t.Fatalf("read %s: %v", queryDir, err)
	}
	var out []jobsQuery
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		path := filepath.Join(queryDir, entry.Name())
		src, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, stmt := range splitNamedQueries(string(src)) {
			body := strings.ToLower(stripSQLComments(stmt.body))
			if !reReadsJobs.MatchString(body) {
				continue
			}
			out = append(out, jobsQuery{
				file:                path,
				name:                stmt.name,
				scopedToOpen:        scopesToOpenPostings(body),
				hasPrivatePredicate: rePrivatePred.MatchString(body),
			})
		}
	}
	return out
}

// openCatalogueReads narrows that to the statements restricted to OPEN postings.
func openCatalogueReads(t *testing.T) []jobsQuery {
	t.Helper()
	var out []jobsQuery
	for _, q := range allNamedJobsQueries(t) {
		if q.scopedToOpen {
			out = append(out, q)
		}
	}
	return out
}

// reProjectedOpen is `closed_at IS NULL` parenthesised on its own — the shape of a
// projection like `(j.closed_at IS NULL)::bool AS job_open`, which REPORTS whether a row
// is open instead of restricting the read to open rows.
var reProjectedOpen = regexp.MustCompile(`\(\s*(\w+\.)?closed_at\s+is\s+null\s*\)`)

// scopesToOpenPostings reports whether a statement restricts what it reads to OPEN
// postings — the question this rule is about. Judged per line, because the same three
// words mean something else in four other places, and each of those is a live statement
// here rather than a hypothetical:
//
//	count(*) FILTER (WHERE closed_at IS NULL)      an aggregate over all rows
//	ORDER BY (j.closed_at IS NULL) DESC            a preference, not a filter
//	LEFT JOIN jobs c ON … AND c.closed_at IS NULL  a condition on a second copy of the table
//	(j.closed_at IS NULL)::bool AS job_open        a projection reporting the state
//
// The direction of a miss is deliberate: a line this fails to recognise drops a statement
// OUT of the population, so the detector is worth exactly as much as the population count
// the test asserts beside it, and no more.
func scopesToOpenPostings(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		if !reOpenScope.MatchString(line) || reProjectedOpen.MatchString(line) {
			continue
		}
		if strings.Contains(line, "filter") || strings.Contains(line, "order by") ||
			strings.Contains(line, "join") || strings.Contains(line, "select") {
			continue
		}
		return true
	}
	return false
}
