package atsboard

import "testing"

func TestRegionRequiresTheExplicitLeverEUHost(t *testing.T) {
	for _, raw := range []string{"https://JOBS.EU.LEVER.CO/acme/job", "https://Api.Eu.Lever.Co/v0/postings/acme/job", "https://jobs.eu.lever.co/acme/job", "https://api.eu.lever.co/v0/postings/acme/job"} {
		if Region(raw) != "eu" {
			t.Fatalf("lost EU data region: %s", raw)
		}
	}
	if Region("https://jobs.lever.co/acme/job") != "" || Region("https://jobs.eu.lever.co.example.com/acme") != "" {
		t.Fatal("inferred a data region from an unrelated host")
	}
}
