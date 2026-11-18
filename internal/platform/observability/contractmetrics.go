package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// A contract route answers a body or params its schema rejects with 400, which is
// the client's fault and never reported as a fault. When our own web client sends
// that shape, every user of the route fails silently; this counter lets an alert
// see a route that suddenly rejects.
var contractRejects = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "freehire_contract_rejects_total",
	Help: "Contract requests refused by the generated decoder, by operation id and kind (body, params).",
}, []string{"operation", "kind"})

// RecordContractReject counts one decoder refusal. operation is the contract's
// operation id, a closed set, so it is safe as a label.
func RecordContractReject(operation, kind string) {
	contractRejects.WithLabelValues(operation, kind).Inc()
}
