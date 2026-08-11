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
		_, _ = w.Write([]byte(`{"id":"1","title":"Engineer","description":"..."}`))
	}))
	defer srv.Close()

	verdict, _ := checkEchoJobsLiveAt(context.Background(), srv.Client(), srv.URL+"/%s", "some-handle")
	if verdict != liveness.Live {
		t.Errorf("verdict = %v, want Live", verdict)
	}
}

func TestCheckEchoJobsLiveOnRemovedPosting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"error":"job fetch failed"}`))
	}))
	defer srv.Close()

	verdict, reason := checkEchoJobsLiveAt(context.Background(), srv.Client(), srv.URL+"/%s", "gone-handle")
	if verdict != liveness.Expired {
		t.Errorf("verdict = %v, want Expired", verdict)
	}
	if reason != "echojobs_detail_gone" {
		t.Errorf("reason = %q, want echojobs_detail_gone", reason)
	}
}

func TestCheckEchoJobsLiveOnUnrelatedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"error":"internal error"}`))
	}))
	defer srv.Close()

	// A DIFFERENT error message must not read as "gone" — only the exact known signal
	// does, so a transient/unrelated API failure stays Uncertain (under-closing bias).
	verdict, _ := checkEchoJobsLiveAt(context.Background(), srv.Client(), srv.URL+"/%s", "some-handle")
	if verdict != liveness.Uncertain {
		t.Errorf("verdict = %v, want Uncertain for an unrelated error body", verdict)
	}
}

func TestCheckEchoJobsLiveStripsBoardlessColonPrefix(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
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
