package sources

import (
	"regexp"
)

// brazilRegimeTerm finds the two contract regimes a Brazilian board's "Tipo de contrato"
// picklist names: "CLT (Efetivo)", "Efetivo – CLT", "Efetivo (CLT)" for employment under
// the labour code, "Prestador de serviços (PJ)" for the contractor's own company. Catho
// can name both on one posting ("CLT (Efetivo) Prestador de serviços (PJ)").
var (
	brazilRegimeCLT = regexp.MustCompile(`(?i)(?:^|[^\pL])clt(?:$|[^\pL])`)
	brazilRegimePJ  = regexp.MustCompile(`(?i)(?:^|[^\pL])pj(?:$|[^\pL])`)
)

// brazilRegimeEngagement maps the regime picklist to vocab.EngagementValues, in its
// order; nil for the regimes that are neither ("Estágio", "Autônomo", "A Combinar").
func brazilRegimeEngagement(regime string) []string {
	var out []string
	if brazilRegimeCLT.MatchString(regime) {
		out = append(out, "clt")
	}
	if brazilRegimePJ.MatchString(regime) {
		out = append(out, "pj")
	}
	return out
}
