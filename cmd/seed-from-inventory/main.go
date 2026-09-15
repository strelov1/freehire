// Command seed-from-inventory converts an external ATS-company inventory CSV — a
// `name,slug,url` file such as kalil0321/ats-scrapers' `ats-companies/<provider>.csv`
// files — into per-provider seed JSON files that cmd/harvest-boards already consumes.
//
// Each row's `url` is resolved to a (provider, board) pair via the existing
// internal/ingest/atsboard.Recognize, the one place freehire's URL-to-board rules live —
// this tool adds no parsing logic of its own. A row atsboard cannot resolve (a vanity
// domain, or a custom-domain ATS such as Taleo, SuccessFactors, or Oracle on its own
// domain) is skipped and counted, never fatal to the run.
//
// It makes no network call and needs no database: the conversion is a pure local
// transform, so its output can be reviewed before ever being handed to
// cmd/harvest-boards, which is the tool that actually probes and writes to the catalog.
//
// Usage:
//
//	go run ./cmd/seed-from-inventory -in ats-companies/workday.csv -out ./seeds
//	go run ./cmd/harvest-boards workday ./seeds/workday.json   # separate, later step
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
)

func main() {
	in := flag.String("in", "", "input CSV file with name,slug,url columns (required)")
	out := flag.String("out", "", "output directory for per-provider seed files (required)")
	flag.Parse()

	if *in == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "seed-from-inventory: -in and -out are required")
		os.Exit(2)
	}

	os.Exit(run(*in, *out, os.Stdout))
}

// run parses the CSV at inPath, resolves and groups its rows, and writes one seed file per
// provider into outDir. It writes nothing and returns non-zero on a structural input
// failure (unreadable file, missing required column, malformed CSV). On success it prints a
// per-provider board count and the total unrecognized-row count to stdout, and returns 0.
func run(inPath, outDir string, stdout io.Writer) int {
	f, err := os.Open(inPath)
	if err != nil {
		return fail(err)
	}
	defer func() { _ = f.Close() }()

	rows, err := parseInventory(f)
	if err != nil {
		return fail(err)
	}

	byProvider, unrecognized := convert(rows)

	if err := writeSeeds(outDir, byProvider); err != nil {
		return fail(err)
	}

	providers := make([]string, 0, len(byProvider))
	for provider := range byProvider {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	for _, provider := range providers {
		// A summary print has no actionable recipient for a write failure — same category
		// errcheck's own exclude-functions list already carries for cleanup calls.
		_, _ = fmt.Fprintf(stdout, "seed-from-inventory: %s: %d boards\n", provider, len(byProvider[provider]))
	}
	_, _ = fmt.Fprintf(stdout, "seed-from-inventory: %d rows unrecognized\n", unrecognized)
	return 0
}

// fail reports err to stderr — matching cmd/harvest-boards and cmd/merge-companies, which
// route both progress and errors there — and returns the exit code run should return for a
// structural failure. The success-path summary stays on the stdout writer run was given.
func fail(err error) int {
	fmt.Fprintf(os.Stderr, "seed-from-inventory: %v\n", err)
	return 1
}
