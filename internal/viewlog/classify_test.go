package viewlog

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		name     string
		rec      Record
		wantOK   bool
		wantSlug string
		wantKind Kind
	}{
		{
			name:     "page open",
			rec:      Record{Method: "GET", Path: "/jobs/acme-eng-123", Status: 200},
			wantOK:   true,
			wantSlug: "acme-eng-123",
			wantKind: KindPage,
		},
		{
			name:     "api read",
			rec:      Record{Method: "GET", Path: "/api/v1/jobs/acme-eng-123", Status: 200},
			wantOK:   true,
			wantSlug: "acme-eng-123",
			wantKind: KindAPI,
		},
		{
			name:     "page open with query string strips query",
			rec:      Record{Method: "GET", Path: "/jobs/acme-eng-123?utm_source=x", Status: 200},
			wantOK:   true,
			wantSlug: "acme-eng-123",
			wantKind: KindPage,
		},
		{
			// SvelteKit client-side (SPA) navigation to the detail page fetches the
			// load data, not the HTML — this is how in-app clicks show up.
			name:     "sveltekit __data.json navigation counts as a page view",
			rec:      Record{Method: "GET", Path: "/jobs/acme-eng-123/__data.json", Status: 200},
			wantOK:   true,
			wantSlug: "acme-eng-123",
			wantKind: KindPage,
		},
		{
			name:     "__data.json with sveltekit query strips both",
			rec:      Record{Method: "GET", Path: "/jobs/acme-eng-123/__data.json?x-sveltekit-invalidated=01", Status: 200},
			wantOK:   true,
			wantSlug: "acme-eng-123",
			wantKind: KindPage,
		},
		{
			name:   "sub-resource __data.json is not a detail-page view",
			rec:    Record{Method: "GET", Path: "/jobs/acme-eng-123/similar/__data.json", Status: 200},
			wantOK: false,
		},
		{
			name:   "job list is not a view",
			rec:    Record{Method: "GET", Path: "/jobs", Status: 200},
			wantOK: false,
		},
		{
			name:   "sub-resource of a job is not a view",
			rec:    Record{Method: "GET", Path: "/jobs/acme-eng-123/similar", Status: 200},
			wantOK: false,
		},
		{
			name:   "api sub-resource is not a view",
			rec:    Record{Method: "GET", Path: "/api/v1/jobs/acme-eng-123/fit/stream", Status: 200},
			wantOK: false,
		},
		{
			name:   "non-GET is not a view",
			rec:    Record{Method: "POST", Path: "/jobs/acme-eng-123", Status: 200},
			wantOK: false,
		},
		{
			name:   "non-2xx is not a view",
			rec:    Record{Method: "GET", Path: "/jobs/acme-eng-123", Status: 404},
			wantOK: false,
		},
		{
			name:   "unrelated path is not a view",
			rec:    Record{Method: "GET", Path: "/companies/acme", Status: 200},
			wantOK: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sig, ok := Classify(tc.rec)
			if ok != tc.wantOK {
				t.Fatalf("Classify ok = %v, want %v", ok, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if sig.Slug != tc.wantSlug {
				t.Errorf("Slug = %q, want %q", sig.Slug, tc.wantSlug)
			}
			if sig.Kind != tc.wantKind {
				t.Errorf("Kind = %v, want %v", sig.Kind, tc.wantKind)
			}
		})
	}
}
