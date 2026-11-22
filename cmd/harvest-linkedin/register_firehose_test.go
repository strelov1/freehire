package main

import (
	"bytes"
	"log"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestWorklistAcceptsFirehoseKeywordAndPerEntryHours(t *testing.T) {
	queries, err := loadQueries(writeTemp(t, `queries:
  - keywords: "*"
    location: United States
    jobage_hours: 3
    pages: 10
matrix:
  - keywords: ["*"]
    locations: [Germany, France]
    jobage_hours: 24
`))
	if err != nil || len(queries) != 3 {
		t.Fatalf("queries=%+v, %v", queries, err)
	}
	if queries[0].Keywords != "*" || queries[0].JobAgeHours != 3 || queries[1].JobAgeHours != 24 || queries[2].Location != "France" {
		t.Fatalf("queries=%+v", queries)
	}
	inputs, err := registrationPlan(queries)
	if err != nil {
		t.Fatal(err)
	}
	if got := inputs[0]; got.Board != "*" || got.Company != "LinkedIn United States all jobs" ||
		!reflect.DeepEqual(got.Tenants, map[string]string{"pages": "10", "jobage": "7", "jobage_hours": "3"}) {
		t.Fatalf("firehose input = %+v", got)
	}
}

func TestWorklistRejectsBadJobAgeHours(t *testing.T) {
	for _, v := range []string{"-1", "721", "1.5", "abc"} {
		if got, err := loadQueries(writeTemp(t, "queries:\n  - keywords: '*'\n    location: Germany\n    jobage_hours: "+v+"\n")); err == nil {
			t.Errorf("accepted jobage_hours=%s: %+v", v, got)
		}
	}
}

func TestRegistrationPlanOmitsJobAgeHoursUnlessSet(t *testing.T) {
	inputs, err := registrationPlan([]query{{Keywords: "go", Location: "UK"}})
	if err != nil || !reflect.DeepEqual(inputs[0].Tenants, map[string]string{"pages": "1", "jobage": "7"}) {
		t.Fatalf("%+v, %v", inputs, err)
	}
}

func TestJobAgeHoursFlagOverridesWorklistLikePagesAndJobAge(t *testing.T) {
	path := writeTemp(t, "queries:\n  - keywords: '*'\n    location: Germany\n    jobage_hours: 24\n")
	for _, tc := range []struct{ flags, want string }{
		{"", "jobage_hours=24 "},
		{"--jobage-hours=6", "jobage_hours=6 "},
		{"--jobage-hours=0", "jobage_hours=24 "},
	} {
		var output bytes.Buffer
		previous := log.Writer()
		log.SetOutput(&output)
		args := []string{"--register-searches"}
		if tc.flags != "" {
			args = append(args, tc.flags)
		}
		exit := runArgs(append(args, path))
		log.SetOutput(previous)
		if exit != 0 || !strings.Contains(output.String(), `keyword="*"`) || !strings.Contains(output.String(), tc.want) {
			t.Errorf("%q: exit=%d, want %q in %s", tc.flags, exit, tc.want, output.String())
		}
	}
}

func TestJobAgeHoursFlagRejectsWrongScopeOrBoundsBeforeReadingTheWorklist(t *testing.T) {
	for _, args := range [][]string{
		{"--jobage-hours=6"},
		{"--register-searches", "--jobage-hours=-1"},
		{"--register-searches", "--jobage-hours=721"},
	} {
		if exit := runArgs(append(args, "/missing/worklist.yml")); exit != 2 {
			t.Errorf("%v returned %d, want usage error", args, exit)
		}
	}
}

func TestSearchURLFirehoseHasNoKeywordsAndUsesHours(t *testing.T) {
	u, err := url.Parse(searchURL(query{Keywords: "*", Location: "Germany", JobAge: 7, JobAgeHours: 6}, 1))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if _, ok := q["keywords"]; ok || q.Get("f_TPR") != "r21600" || q.Get("location") != "Germany" {
		t.Fatalf("query = %v", q)
	}
}
