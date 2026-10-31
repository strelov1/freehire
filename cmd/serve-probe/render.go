package main

import (
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/search/serveprobe"
)

func render(rep serveprobe.Report) string {
	var b strings.Builder
	header(&b, "freehire_serve_probe_gap", "Sampled open postings that STORE a field but do not SERVE it, by layer (projection = the API's own projection, a code bug; index = the live Meilisearch document, a reindex owed when projection is 0).")
	perField(&b, "freehire_serve_probe_gap", rep, func(c serveprobe.Count) int { return c.Gaps })
	header(&b, "freehire_serve_probe_stored", "Sampled open postings that store the field — the denominator of freehire_serve_probe_gap.")
	perField(&b, "freehire_serve_probe_stored", rep, func(c serveprobe.Count) int { return c.Stored })
	header(&b, "freehire_serve_probe_sampled", "Open postings this run sampled.")
	fmt.Fprintf(&b, "freehire_serve_probe_sampled %d\n", rep.Sampled)
	header(&b, "freehire_serve_probe_unindexed", "Sampled postings with no Meilisearch document (skipped by the rebuild's Go-side rules, or not yet drained); not a gap.")
	fmt.Fprintf(&b, "freehire_serve_probe_unindexed %d\n", rep.Unindexed)
	header(&b, "freehire_serve_probe_unreadable", "Sampled postings the projection itself refused to render.")
	fmt.Fprintf(&b, "freehire_serve_probe_unreadable %d\n", rep.Unreadable)
	return b.String()
}

// perField writes one sample per (layer, field) pair, zeros included, so a dashboard
// can tell "no gap" from "never measured".
func perField(b *strings.Builder, name string, rep serveprobe.Report, value func(serveprobe.Count) int) {
	for _, l := range serveprobe.Layers {
		for _, f := range serveprobe.Fields() {
			fmt.Fprintf(b, "%s{layer=%q,field=%q} %d\n", name, l, f, value(rep.Counts[l][f]))
		}
	}
}

func header(b *strings.Builder, name, help string) {
	fmt.Fprintf(b, "# HELP %s %s\n# TYPE %s gauge\n", name, help, name)
}
