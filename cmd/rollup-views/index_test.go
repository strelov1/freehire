package main

import (
	"slices"
	"testing"
)

func TestViewStepStarts(t *testing.T) {
	var starts []int64
	prev := -1
	for n := int64(0); n < 60; n++ {
		if s := viewStep(n); s != prev {
			starts = append(starts, n)
			prev = s
		}
	}
	if want := []int64{0, 5, 7, 8, 10, 13, 16, 20, 24, 30, 38, 47, 59}; !slices.Equal(starts, want) {
		t.Fatalf("steps start at %v, want %v", starts, want)
	}
}
