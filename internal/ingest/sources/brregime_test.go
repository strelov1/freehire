package sources

import (
	"slices"
	"testing"
)

// The picklist values the four Brazilian adapters read, as the boards spell them.
func TestBrazilRegimeEngagement(t *testing.T) {
	cases := map[string][]string{
		"CLT (Efetivo)":                            {"clt"},
		"Efetivo – CLT - Período Integral":         {"clt"},
		"Efetivo (CLT)":                            {"clt"},
		"Prestador de serviços (PJ)":               {"pj"},
		"Prestador de Serviços (PJ)":               {"pj"},
		"CLT (Efetivo) Prestador de serviços (PJ)": {"clt", "pj"},
		"Estágio":    nil,
		"Autônomo":   nil,
		"A Combinar": nil,
	}
	for in, want := range cases {
		if got := brazilRegimeEngagement(in); !slices.Equal(got, want) {
			t.Errorf("brazilRegimeEngagement(%q) = %v, want %v", in, got, want)
		}
	}
}
