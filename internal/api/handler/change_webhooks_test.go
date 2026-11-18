package handler

import (
	"slices"
	"strconv"
	"testing"
)

func TestNormalizeCompanySlugs(t *testing.T) {
	got, err := normalizeCompanySlugs([]string{" Acme ", "acme", "beta"})
	if err != nil || !slices.Equal(got, []string{"acme", "beta"}) {
		t.Fatalf("got %v, %v; want [acme beta]", got, err)
	}
	if _, err := normalizeCompanySlugs([]string{"acme", " "}); err == nil {
		t.Fatal("an empty slug was accepted")
	}
	tooMany := make([]string, maxChangeWebhookCompanies+1)
	for i := range tooMany {
		tooMany[i] = "c" + strconv.Itoa(i)
	}
	if _, err := normalizeCompanySlugs(tooMany); err == nil {
		t.Fatalf("%d slugs were accepted", len(tooMany))
	}
}
