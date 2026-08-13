package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/strelov1/freehire/internal/liveness"
)

func TestCheckEchoJobsLiveOnLivePosting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/some-handle" {
			t.Errorf("path = %q, want /some-handle", r.URL.Path)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	verdict, _ := checkEchoJobsLiveAt(context.Background(), srv.Client(), srv.URL+"/%s", "some-handle")
	if verdict != liveness.Live {
		t.Errorf("verdict = %v, want Live", verdict)
	}
}

func TestCheckEchoJobsLiveOnRemovedPosting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()

	verdict, reason := checkEchoJobsLiveAt(context.Background(), srv.Client(), srv.URL+"/%s", "gone-handle")
	if verdict != liveness.Expired {
		t.Errorf("verdict = %v, want Expired", verdict)
	}
	if reason != "echojobs_job_gone" {
		t.Errorf("reason = %q, want echojobs_job_gone", reason)
	}
}

// 410 Gone is the same "removed" signal as 404 for every other liveness probe in this worker
// (see internal/liveness.Classify) — echojobs' own probe must not treat it as merely Uncertain.
func TestCheckEchoJobsLiveOnGonePosting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(410)
	}))
	defer srv.Close()

	verdict, reason := checkEchoJobsLiveAt(context.Background(), srv.Client(), srv.URL+"/%s", "gone-handle")
	if verdict != liveness.Expired {
		t.Errorf("verdict = %v, want Expired", verdict)
	}
	if reason != "echojobs_job_gone" {
		t.Errorf("reason = %q, want echojobs_job_gone", reason)
	}
}

func TestCheckEchoJobsLiveOnUnrelatedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	// A non-200/404 status must not read as "gone" — only the exact known-removed signal
	// does, so a transient/unrelated failure stays Uncertain (under-closing bias).
	verdict, _ := checkEchoJobsLiveAt(context.Background(), srv.Client(), srv.URL+"/%s", "some-handle")
	if verdict != liveness.Uncertain {
		t.Errorf("verdict = %v, want Uncertain for a non-200/404 status", verdict)
	}
}

func TestCheckEchoJobsLiveStripsBoardlessColonPrefix(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(200)
	}))
	defer srv.Close()

	verdict, _ := checkEchoJobsLiveAt(context.Background(), srv.Client(), srv.URL+"/%s", trimEchoJobsHandle(":some-handle"))
	if verdict != liveness.Live {
		t.Fatalf("verdict = %v, want Live", verdict)
	}
	if gotPath != "/some-handle" {
		t.Errorf("requested path = %q, want /some-handle (leading ':' stripped)", gotPath)
	}
}
