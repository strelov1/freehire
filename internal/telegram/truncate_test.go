package telegram

import (
	"strings"
	"testing"
)

func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("hello", 10); got != "hello" {
		t.Errorf("short input changed: %q", got)
	}
	long := strings.Repeat("я", 100) // multi-byte runes
	got := truncateRunes(long, 10)
	if n := len([]rune(got)); n != 10 {
		t.Errorf("rune length = %d, want 10", n)
	}
	if strings.ContainsRune(got, '�') {
		t.Error("truncation split a multi-byte rune")
	}
}
