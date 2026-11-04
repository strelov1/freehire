package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var registrations = promauto.NewCounter(prometheus.CounterOpts{
	Name: "freehire_registrations_total",
	Help: "Committed new accounts, excluding logins and identity links.",
})

var cvUploads = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "freehire_cv_uploads_total",
	Help: "CV storage attempts after validation, by persistence outcome. Excludes guest and stateless parsing.",
}, []string{"outcome"})

var cvExtractions = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "freehire_cv_extractions_total",
	Help: "Background CV extraction attempts (including retries), by outcome.",
}, []string{"outcome"})

func init() {
	// An idle product must expose zeros, so no events and no telemetry are distinguishable.
	for _, outcome := range []string{"success", "error"} {
		cvUploads.WithLabelValues(outcome)
		cvExtractions.WithLabelValues(outcome)
	}
}

func RecordRegistration() { registrations.Inc() }

func RecordCVUpload(success bool) {
	cvUploads.WithLabelValues(productOutcome(success)).Inc()
}

func RecordCVExtraction(success bool) {
	cvExtractions.WithLabelValues(productOutcome(success)).Inc()
}

func productOutcome(success bool) string {
	if success {
		return "success"
	}
	return "error"
}
