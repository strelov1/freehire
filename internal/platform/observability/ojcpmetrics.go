package observability

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var ojcpRESTRequests = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "freehire_ojcp_rest_requests_total", Help: "OJCP REST responses by fixed operation and final HTTP status; excludes MCP.",
}, []string{"operation", "status"})

var ojcpRESTDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name: "freehire_ojcp_rest_duration_seconds", Help: "OJCP REST response latency, including error rendering.",
	Buckets: []float64{0.005, 0.025, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
}, []string{"operation"})

var ojcpRESTBytes = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name: "freehire_ojcp_rest_response_bytes", Help: "OJCP REST response body bytes before compression; no payloads retained.",
	Buckets: []float64{1024, 4096, 16384, 65536, 262144, 1048576, 2097152, 4194304, 8388608, 16777216, 33554432, 67108864},
}, []string{"operation"})

// Called at the shared final-response boundary, including returned errors and recovered panics.
// Match registered route patterns only: a caller's slug must never become a metric label.
func observeOJCPResponse(c *fiber.Ctx, route string) {
	var operation string
	switch route {
	case "/api/v1/ojcp/v1/search":
		operation = "search_jobs"
	case "/api/v1/ojcp/v1/jobs/:slug":
		operation = "get_job_detail"
	case "/api/v1/ojcp/v1/employers/:slug":
		operation = "get_employer_context"
	case "/api/v1/ojcp/manifest":
		operation = "manifest"
	case "/api/v1/ojcp/keys":
		operation = "keys"
	default:
		return
	}
	ojcpRESTRequests.WithLabelValues(operation, strconv.Itoa(c.Response().StatusCode())).Inc()
	ojcpRESTDuration.WithLabelValues(operation).Observe(time.Since(c.Context().Time()).Seconds())
	ojcpRESTBytes.WithLabelValues(operation).Observe(float64(len(c.Response().Body())))
}
