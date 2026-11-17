package main

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"testing"
)

func TestRegistrationPreviewCanOverrideNewSearchBudgets(t *testing.T) {
	for _, tc := range []struct {
		name       string
		flags      []string
		pages, age int
	}{
		{"worklist defaults", nil, 2, 14},
		{"both overrides", []string{"--pages=10", "--jobage=30"}, 10, 30},
		{"page override", []string{"--pages=10"}, 10, 14},
		{"age override", []string{"--jobage=30"}, 2, 30},
		{"explicit zero retains worklist", []string{"--pages=0", "--jobage=0"}, 2, 14},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "not-a-database")
			t.Setenv("SCRAPLING_FETCH_URL", "not-a-service")
			path := writeTemp(t, "queries:\n  - keywords: software engineer\n    location: Germany\n    pages: 2\n    jobage: 14\n")
			var output bytes.Buffer
			previous := log.Writer()
			log.SetOutput(&output)
			t.Cleanup(func() { log.SetOutput(previous) })
			args := append([]string{"--register-searches"}, tc.flags...)
			args = append(args, path)
			if exit := runArgs(args); exit != 0 {
				t.Fatalf("preview exit=%d, log=%s", exit, output.String())
			}
			if want := fmt.Sprintf("pages=%d jobage=%d", tc.pages, tc.age); !strings.Contains(output.String(), want) {
				t.Fatalf("preview did not apply %s: %s", want, output.String())
			}
		})
	}
}

func TestRegistrationBudgetFlagsRejectWrongScopeOrBoundsBeforeReadingTheWorklist(t *testing.T) {
	for _, args := range [][]string{
		{"--pages=0"}, {"--jobage=0"}, {"--pages=10"}, {"--jobage=30"},
		{"--register-searches", "--apply", "--pages=-1"},
		{"--register-searches", "--apply", "--pages=101"},
		{"--register-searches", "--apply", "--jobage=-1"},
		{"--register-searches", "--apply", "--jobage=31"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			if exit := runArgs(append(args, "/missing/worklist.yml")); exit != 2 {
				t.Fatalf("invalid registration flags returned %d, want usage error before file/DB access", exit)
			}
		})
	}
}
