package main

import "testing"

func TestParseLine(t *testing.T) {
	cases := []struct {
		name, line, wantCompany, wantURL string
	}{
		{
			name:        "company and url tab-separated",
			line:        "Acme Corp\thttps://acme.example/careers",
			wantCompany: "Acme Corp",
			wantURL:     "https://acme.example/careers",
		},
		{
			name:        "bare url uses host as company",
			line:        "https://www.acme.example/careers",
			wantCompany: "acme.example",
			wantURL:     "https://www.acme.example/careers",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			company, u := parseLine(c.line)
			if company != c.wantCompany || u != c.wantURL {
				t.Errorf("parseLine(%q) = (%q, %q), want (%q, %q)", c.line, company, u, c.wantCompany, c.wantURL)
			}
		})
	}
}

func TestExtractCfg(t *testing.T) {
	cases := []struct {
		name, page, want string
	}{
		{
			name: "script src widget",
			page: `<html><body><script src="https://skk.erecruiter.pl/Code.ashx?cfg=eac02250aa184ed4bf4950e74948549a"></script></body></html>`,
			want: "eac02250aa184ed4bf4950e74948549a",
		},
		{
			name: "protocol-relative and mixed case",
			page: `<script src="//skk.erecruiter.pl/Code.ashx?cfg=ABC123def4567890abcdef1234567890"></script>`,
			want: "ABC123def4567890abcdef1234567890",
		},
		{
			name: "no erecruiter widget",
			page: `<html><body><a href="https://boards.greenhouse.io/acme">Careers</a></body></html>`,
			want: "",
		},
		{
			name: "empty page",
			page: "",
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := extractCfg(c.page); got != c.want {
				t.Errorf("extractCfg() = %q, want %q", got, c.want)
			}
		})
	}
}
