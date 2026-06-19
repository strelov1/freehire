package main

import (
	"errors"
	"testing"
)

func TestCareersLink(t *testing.T) {
	cases := []struct {
		name string
		html string
		base string
		want string
	}{
		{
			name: "relative careers href resolved against base",
			html: `<nav><a href="/company/careers">Careers</a></nav>`,
			base: "https://acme.com",
			want: "https://acme.com/company/careers",
		},
		{
			name: "absolute jobs href by link text",
			html: `<a href="https://jobs.acme.com">Open Roles</a><a href="https://acme.com/jobs">Jobs</a>`,
			base: "https://acme.com",
			want: "https://jobs.acme.com",
		},
		{
			name: "no careers link",
			html: `<a href="/about">About</a><a href="/blog">Blog</a>`,
			base: "https://acme.com",
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := careersLink(tc.html, tc.base); got != tc.want {
				t.Errorf("careersLink = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	t.Run("detects on a careers path the homepage links to nothing", func(t *testing.T) {
		pages := map[string]string{
			"https://acme.com":         `<a href="/careers">Careers</a>`,
			"https://acme.com/careers": `<a href="https://jobs.lever.co/acme">Apply</a>`,
		}
		fetch := func(u string) (string, error) { return pages[u], nil }
		p, s, ok := resolve("https://acme.com", fetch)
		if !ok || p != "lever" || s != "acme" {
			t.Fatalf("resolve = (%q,%q,%v), want (lever,acme,true)", p, s, ok)
		}
	})

	t.Run("detects directly on the homepage", func(t *testing.T) {
		fetch := func(u string) (string, error) {
			if u == "https://acme.com" {
				return `<script src="https://boards.greenhouse.io/embed/job_board/js?for=acme"></script>`, nil
			}
			return "", nil
		}
		p, s, ok := resolve("https://acme.com", fetch)
		if !ok || p != "greenhouse" || s != "acme" {
			t.Fatalf("resolve = (%q,%q,%v), want (greenhouse,acme,true)", p, s, ok)
		}
	})

	t.Run("no ats anywhere yields ok=false", func(t *testing.T) {
		fetch := func(u string) (string, error) { return `<p>hiring soon</p>`, nil }
		if _, _, ok := resolve("https://acme.com", fetch); ok {
			t.Fatal("resolve ok = true, want false")
		}
	})

	t.Run("dead homepage skips career-path probes", func(t *testing.T) {
		var calls int
		fetch := func(u string) (string, error) {
			calls++
			if u == "https://acme.com" {
				return "", errors.New("dial tcp: no such host")
			}
			return `<a href="https://jobs.lever.co/acme">Jobs</a>`, nil // would resolve if probed
		}
		if _, _, ok := resolve("https://acme.com", fetch); ok {
			t.Fatal("resolve ok = true, want false (homepage was dead)")
		}
		if calls != 1 {
			t.Errorf("fetch called %d times, want 1 (no path probes after a dead homepage)", calls)
		}
	})
}
