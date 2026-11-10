package main

import (
	"context"
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/strelov1/freehire/internal/search/search"
)

func TestFacetModeSource(t *testing.T) {
	var val string
	var err error
	s := newFacetModeSource(search.FacetModeShadow, func(context.Context) (string, error) { return val, err })
	ctx := context.Background()
	check := func(step string, want search.FacetMode) {
		t.Helper()
		if got := s.Mode(); got != want {
			t.Errorf("%s: mode = %q, want %q", step, got, want)
		}
	}
	check("start", search.FacetModeShadow)

	val = "facetd"
	s.refresh(ctx)
	check("override", search.FacetModeFacetd)

	val = "bogus"
	s.refresh(ctx)
	check("invalid ignored", search.FacetModeFacetd)

	val, err = "", errors.New("redis down")
	s.refresh(ctx)
	check("redis error keeps last", search.FacetModeFacetd)

	err = redis.Nil
	s.refresh(ctx)
	check("absent key returns to start", search.FacetModeShadow)
}
