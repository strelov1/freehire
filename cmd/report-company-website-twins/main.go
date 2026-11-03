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
	"cmp"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"slices"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/job/logodomain"
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

	rows, err := db.New(pool).ListCompanyWebsites(ctx)
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

type group struct {
	Host    string
	Members []db.ListCompanyWebsitesRow
}

// twinGroups groups the companies by the domain their website names, through the same
// logodomain.Domain the logo map uses, so the two never disagree about a shared site. It
// keeps the groups of two or more that the name fold keeps apart, which are what a person
// has to judge, biggest employer first within a group.
func twinGroups(rows []db.ListCompanyWebsitesRow) []group {
	byHost := make(map[string][]db.ListCompanyWebsitesRow)
	for _, r := range rows {
		if host := logodomain.Domain(r.Website); host != "" {
			byHost[host] = append(byHost[host], r)
		}
	}
	var out []group
	for host, members := range byHost {
		if len(members) < 2 || oneFold(members) {
			continue
		}
		slices.SortFunc(members, func(a, b db.ListCompanyWebsitesRow) int {
			if d := cmp.Compare(b.JobCount, a.JobCount); d != 0 {
				return d
			}
			return cmp.Compare(a.Slug, b.Slug)
		})
		out = append(out, group{Host: host, Members: members})
	}
	slices.SortFunc(out, func(a, b group) int { return cmp.Compare(a.Host, b.Host) })
	return out
}

func oneFold(ms []db.ListCompanyWebsitesRow) bool {
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
