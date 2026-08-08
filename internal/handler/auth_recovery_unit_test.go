package handler

import (
	"testing"
)

func TestMaskEmail(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"user@example.com", "u***@e***.com"},
		{"john.doe@company.org", "j***@c***.org"},
		{"a@b.com", "a***@b***.com"},
		{"test@localhost", "t***@l***"},
		{"invalid-email", "***"},
		{"", "***"},
		{"@domain.com", "***"},
		{"user@", "***"},
	}

	for _, tt := range tests {
		got := maskEmail(tt.input)
		if got != tt.want {
			t.Errorf("maskEmail(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
