package vote

import (
	"errors"
	"testing"
)

func TestParseDirection(t *testing.T) {
	cases := []struct {
		in   string
		want Direction
		err  bool
	}{
		{"up", Up, false},
		{"down", Down, false},
		{"", 0, true},
		{"UP", 0, true},
		{"1", 0, true},
		{"neutral", 0, true},
	}
	for _, c := range cases {
		got, err := ParseDirection(c.in)
		if c.err {
			if !errors.Is(err, ErrInvalidDirection) {
				t.Errorf("ParseDirection(%q): want ErrInvalidDirection, got %v", c.in, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("ParseDirection(%q) = %d, %v; want %d, nil", c.in, got, err, c.want)
		}
	}
}
