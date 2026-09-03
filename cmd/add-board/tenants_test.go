package main

import (
	"reflect"
	"testing"
)

func TestParseTenantsEmpty(t *testing.T) {
	got, err := parseTenants("")
	if err != nil {
		t.Fatalf("parseTenants(\"\"): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("parseTenants(\"\") = %v, want empty", got)
	}
}

func TestParseTenantsPairs(t *testing.T) {
	got, err := parseTenants("Arvato_Systems:Arvato Systems,acme:Acme Inc")
	if err != nil {
		t.Fatalf("parseTenants: %v", err)
	}
	want := map[string]string{"Arvato_Systems": "Arvato Systems", "acme": "Acme Inc"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseTenants = %v, want %v", got, want)
	}
}

func TestParseTenantsRejectsMalformedPair(t *testing.T) {
	if _, err := parseTenants("no-colon-here"); err == nil {
		t.Fatal("expected error for a pair with no ':', got nil")
	}
}
