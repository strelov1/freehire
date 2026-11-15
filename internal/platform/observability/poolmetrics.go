package observability

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// poolStatter is the slice of *pgxpool.Pool these gauges read. An interface rather than the
// concrete type so the collector can be driven in a test without opening a database.
type poolStatter interface {
	Stat() *pgxpool.Stat
}

// poolCollector publishes the connection pool's occupancy.
//
// Pool occupancy matters because a crawl holding every connection leaves pool.Ping fast and the
// error fraction clean (few responses are produced), so no other signal shows it.
//
// A Collector, not a polled gauge: Stat() reads in-memory counters, so scrape-time reads are
// cheaper and fresher, with no interval or ticker to manage.
type poolCollector struct {
	pool poolStatter

	acquired  *prometheus.Desc
	idle      *prometheus.Desc
	max       *prometheus.Desc
	emptyWait *prometheus.Desc
	waited    *prometheus.Desc
}

// NewPoolCollector builds the collector for one pool. Register it on the default registry
// with prometheus.MustRegister; cmd/server does.
func NewPoolCollector(pool poolStatter) prometheus.Collector {
	return &poolCollector{
		pool: pool,
		acquired: prometheus.NewDesc("freehire_db_pool_acquired_connections",
			"Connections currently held by a caller.", nil, nil),
		idle: prometheus.NewDesc("freehire_db_pool_idle_connections",
			"Connections open and free to be handed out.", nil, nil),
		max: prometheus.NewDesc("freehire_db_pool_max_connections",
			"The pool's ceiling. Saturation is acquired/max; alert on the ratio, never on the raw count — the ceiling is per-process configuration and changes.", nil, nil),
		emptyWait: prometheus.NewDesc("freehire_db_pool_empty_acquire_total",
			"Acquisitions that found no free connection, however briefly. A trend, not an alerting signal — alert on sustained acquired/max instead.", nil, nil),
		waited: prometheus.NewDesc("freehire_db_pool_acquire_seconds_total",
			"Cumulative time spent inside Acquire, waiting and instant hand-offs alike. A diagnostic: divide by the empty-acquire count for a mean wait. Alert on sustained acquired/max instead.", nil, nil),
	}
}

func (c *poolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.acquired
	ch <- c.idle
	ch <- c.max
	ch <- c.emptyWait
	ch <- c.waited
}

func (c *poolCollector) Collect(ch chan<- prometheus.Metric) {
	stat := c.pool.Stat()
	ch <- prometheus.MustNewConstMetric(c.acquired, prometheus.GaugeValue, float64(stat.AcquiredConns()))
	ch <- prometheus.MustNewConstMetric(c.idle, prometheus.GaugeValue, float64(stat.IdleConns()))
	ch <- prometheus.MustNewConstMetric(c.max, prometheus.GaugeValue, float64(stat.MaxConns()))
	ch <- prometheus.MustNewConstMetric(c.emptyWait, prometheus.CounterValue, float64(stat.EmptyAcquireCount()))
	ch <- prometheus.MustNewConstMetric(c.waited, prometheus.CounterValue, stat.AcquireDuration().Seconds())
}
