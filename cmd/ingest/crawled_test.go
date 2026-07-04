package main

import (
	"slices"
	"sync"
	"testing"
)

func TestCrawledSetRecordsDistinctSlugsPerProvider(t *testing.T) {
	c := newCrawledSet()
	c.record("workable", "jalasoft")
	c.record("workable", "gigabrands")
	c.record("workable", "jalasoft") // duplicate collapses
	c.record("workable", "")         // empty slug is ignored (can't scope to it)
	c.record("greenhouse", "arena-im")

	if got, want := c.slugs("workable"), []string{"gigabrands", "jalasoft"}; !slices.Equal(got, want) {
		t.Errorf("slugs(workable) = %v, want %v (sorted, deduped, no empty)", got, want)
	}
	if got, want := c.slugs("greenhouse"), []string{"arena-im"}; !slices.Equal(got, want) {
		t.Errorf("slugs(greenhouse) = %v, want %v", got, want)
	}
	if got := c.slugs("lever"); len(got) != 0 {
		t.Errorf("slugs(untouched provider) = %v, want empty", got)
	}
}

func TestCrawledSetConcurrentRecord(t *testing.T) {
	c := newCrawledSet()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.record("workable", "acme")
		}()
	}
	wg.Wait()
	if got, want := c.slugs("workable"), []string{"acme"}; !slices.Equal(got, want) {
		t.Errorf("slugs after concurrent record = %v, want %v", got, want)
	}
}
