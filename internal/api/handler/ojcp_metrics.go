package handler

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/strelov1/freehire/internal/api/ojcp"
)

var ojcpRESTSearchQuality = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "freehire_ojcp_rest_search_quality_total", Help: "Successful OJCP REST searches by fixed quality signal; signals may overlap.",
}, []string{"signal"})

func observeOJCPRESTSearch(resp ojcp.SearchJobsResponse) {
	ojcpRESTSearchQuality.WithLabelValues("all").Inc()
	if len(resp.Jobs) == 0 {
		ojcpRESTSearchQuality.WithLabelValues("empty").Inc()
	}
	if len(resp.IgnoredParams) > 0 {
		ojcpRESTSearchQuality.WithLabelValues("ignored_filters").Inc()
	}
	switch resp.Semantic {
	case ojcp.SemanticApplied, ojcp.SemanticUnavailable:
		ojcpRESTSearchQuality.WithLabelValues("semantic_requested").Inc()
		if resp.Semantic == ojcp.SemanticUnavailable {
			ojcpRESTSearchQuality.WithLabelValues("semantic_fallback").Inc()
		}
	}
}
