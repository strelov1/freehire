package outboundurl

import "testing"

func TestTag(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "no query string appends with ?",
			in:   "https://ats.example.com/job/123",
			want: "https://ats.example.com/job/123?utm_source=freehire.me",
		},
		{
			name: "existing query appends with &",
			in:   "https://ats.example.com/job?id=123",
			want: "https://ats.example.com/job?id=123&utm_source=freehire.me",
		},
		{
			name: "existing utm_source is overwritten",
			in:   "https://ats.example.com/job?utm_source=indeed",
			want: "https://ats.example.com/job?utm_source=freehire.me",
		},
		{
			name: "fragment is preserved after the query",
			in:   "https://ats.example.com/job#apply",
			want: "https://ats.example.com/job?utm_source=freehire.me#apply",
		},
		{
			name: "empty url is returned unchanged",
			in:   "",
			want: "",
		},
		{
			name: "unparseable url is returned unchanged",
			in:   "https://ats.example.com/%zz",
			want: "https://ats.example.com/%zz",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Tag(tt.in); got != tt.want {
				t.Errorf("Tag(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
