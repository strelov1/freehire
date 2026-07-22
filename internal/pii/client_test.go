package pii

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPDetectorParsesSpans(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"spans":[{"start":0,"end":4,"kind":"NAME"},{"start":10,"end":20,"kind":"ADDRESS"}]}`))
	}))
	defer srv.Close()

	d := NewHTTPDetector(srv.URL, srv.Client())
	spans, err := d.Detect(context.Background(), "Alex lives at 5th Avenue")
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(spans) != 2 || spans[0].Kind != KindName || spans[1].Kind != KindAddress {
		t.Fatalf("unexpected spans: %+v", spans)
	}
	if spans[0].Start != 0 || spans[0].End != 4 {
		t.Fatalf("bad first span: %+v", spans[0])
	}
}

func TestHTTPDetectorErrorsOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	d := NewHTTPDetector(srv.URL, srv.Client())
	if _, err := d.Detect(context.Background(), "text"); err == nil {
		t.Fatal("expected error on non-200 response, got nil")
	}
}

func TestHTTPDetectorErrorsOnTransport(t *testing.T) {
	d := NewHTTPDetector("http://127.0.0.1:1/detect", http.DefaultClient)
	if _, err := d.Detect(context.Background(), "text"); err == nil {
		t.Fatal("expected transport error, got nil")
	}
}
