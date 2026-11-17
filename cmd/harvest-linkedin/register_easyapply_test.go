package main

import (
	"bytes"
	"log"
	"reflect"
	"strings"
	"testing"
)

func TestRegistrationPlanStoresEasyApplyPagesOnlyWhenRequested(t *testing.T) {
	for _, tc := range []struct {
		pages int
		want  map[string]string
	}{
		{0, map[string]string{"pages": "3", "jobage": "14"}},
		{40, map[string]string{"pages": "3", "jobage": "14", "easyapply_pages": "40"}},
	} {
		inputs, err := registrationPlan([]query{{Keywords: "go", Location: "UK", Pages: 3, JobAge: 14, EasyApplyPages: tc.pages}})
		if err != nil || len(inputs) != 1 || !reflect.DeepEqual(inputs[0].Tenants, tc.want) {
			t.Fatalf("easyapply_pages=%d: %+v, %v; want tenants %v", tc.pages, inputs, err, tc.want)
		}
	}
}

func TestRegistrationPreviewAppliesEasyApplyFlag(t *testing.T) {
	path := writeTemp(t, "queries:\n  - keywords: go\n    location: UK\n")
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previous) })
	if exit := runArgs([]string{"--register-searches", "--easyapply-pages=40", path}); exit != 0 {
		t.Fatalf("preview exit=%d, log=%s", exit, output.String())
	}
	if !strings.Contains(output.String(), "easyapply_pages=40") {
		t.Fatalf("preview did not show the option: %s", output.String())
	}
}

func TestEasyApplyFlagRejectsWrongScopeOrBoundsBeforeReadingTheWorklist(t *testing.T) {
	for _, args := range [][]string{
		{"--easyapply-pages=40"},
		{"--register-searches", "--easyapply-pages=-1"},
		{"--register-searches", "--easyapply-pages=101"},
	} {
		if exit := runArgs(append(args, "/missing/worklist.yml")); exit != 2 {
			t.Errorf("%v returned %d, want a usage error before file/DB access", args, exit)
		}
	}
}
