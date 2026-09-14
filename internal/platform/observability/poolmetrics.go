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
// Nothing in this repository read pool.Stat() before the 2026-09-14 outage, which is the
// reason that outage was invisible. A deep-offset crawl held all ten of the API's connections
// for minutes at a time; every other request queued for one, and the signals that existed all
// looked clean — pool.Ping answers in microseconds while every connection is held, and the
// error fraction counts only responses the process PRODUCED, of which there were almost none
// because nothing was finishing.
//
// Implemented as a Collector rather than gauges a goroutine polls: Stat() reads counters the
// pool already keeps in memory, so reading them at scrape time is both cheaper and fresher
// than sampling on a timer, and there is no interval to pick or ticker to stop.
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
		// How OFTEN a caller found nothing free. Useful as a trend, and deliberately NOT
		// what to alert on — measured on production 2026-09-14 this sits around 15/s on a
		// perfectly healthy pool (1 connection acquired, 9 idle at the same instant),
		// because it counts an acquire that waited at all, including the microseconds pgx
		// spends handing a connection over under ordinary concurrency. An alert on this
		// rate fires permanently, and a permanently red alert is one nobody reads.
		emptyWait: prometheus.NewDesc("freehire_db_pool_empty_acquire_total",
			"Acquisitions that found no free connection, however briefly. A trend, not an alerting signal — see freehire_db_pool_acquire_wait_seconds_total.", nil, nil),
		// How long callers spent inside Acquire, in total. A diagnostic, and NOT the
		// alerting signal either — the name pgx gives it is AcquireDuration and what it
		// documents is "the total duration of all successful Acquire calls", so it counts
		// handing over an already-idle connection just as it counts a two-minute wait.
		// Measured on production it runs at ~1.36 seconds per second against a pool one
		// tenth occupied, which is not 1.36 callers queued; it is Little's law over every
		// acquire, most of them instant.
		//
		// It earns its place as the thing that separates ten thousand waits of a
		// microsecond from ten waits of two minutes, which no count can: divide it by
		// freehire_db_pool_empty_acquire_total for the mean. What carries the ALERT is
		// sustained occupancy — avg_over_time of acquired/max — because during the
		// 2026-09-14 outage the pool sat at 10/10 for fifty minutes, while a healthy pool
		// samples at 0/10 most of the time and touches its ceiling only in bursts.
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
