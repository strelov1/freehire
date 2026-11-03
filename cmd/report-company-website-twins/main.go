// Command report-company-website-twins is a hand-run, read-only report of merge candidates:
// companies with open jobs that state the same website host under different slugs
// (freehire#3895). It writes nothing and is on no timer. A person reads the list and proposes
// the merges from the company page, where a moderator decides them
// (internal/job/companyrelation).
//
// A shared host is evidence, not a decision. A staffing agency's branches ("Forum Jobs
// Mechelen", "Forum Jobs Tielt", all forumjobs.be) are one employer. A job board's or a
// parent group's host can stand behind several. That judgement is why this is a report and
// not a merge pass.
//
// Groups whose names already fold to one key are left out: cmd/merge-companies reaches those
// by itself. The websites come from company_info.website, filled by curated sources and, since
// freehire#3895, by adapters that state the employer's site (EURES).
//
// Needs only DATABASE_URL.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() { worker.Main(run) }

func run() int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	rows, err := db.New(pool).ListCompanyWebsiteTwins(ctx)
	if err != nil {
		log.Printf("report-company-website-twins: %v", err)
		return 1
	}
	groups := twinGroups(rows)
	if err := report(os.Stdout, groups); err != nil {
		log.Printf("report-company-website-twins: write: %v", err)
		return 1
	}
	log.Printf("report-company-website-twins: %d hosts shared by companies the name fold keeps apart", len(groups))
	return 0
}

type member struct {
	Slug     string
	Name     string
	JobCount int32
}

type group struct {
	Host    string
	Members []member
}

// twinGroups groups the rows (already ordered by host) and drops each group the name fold
// already collapses, keeping only what a person has to judge.
func twinGroups(rows []db.ListCompanyWebsiteTwinsRow) []group {
	var out []group
	for i := 0; i < len(rows); {
		g := group{Host: rows[i].Host}
		for ; i < len(rows) && rows[i].Host == g.Host; i++ {
			g.Members = append(g.Members, member{Slug: rows[i].Slug, Name: rows[i].Name, JobCount: rows[i].JobCount})
		}
		if len(g.Members) > 1 && !oneFold(g.Members) {
			out = append(out, g)
		}
	}
	return out
}

func oneFold(ms []member) bool {
	key := normalize.CompanyKey(ms[0].Name)
	for _, m := range ms[1:] {
		if normalize.CompanyKey(m.Name) != key {
			return false
		}
	}
	return true
}

func report(w io.Writer, groups []group) error {
	for _, g := range groups {
		if _, err := fmt.Fprintf(w, "%s\n", g.Host); err != nil {
			return err
		}
		for _, m := range g.Members {
			if _, err := fmt.Fprintf(w, "  %-50s %6d  %s\n", m.Slug, m.JobCount, m.Name); err != nil {
				return err
			}
		}
	}
	return nil
}
