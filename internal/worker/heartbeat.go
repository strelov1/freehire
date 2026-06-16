package worker

import "time"

// Heartbeat calls report every interval until the returned stop is invoked, on a
// background goroutine. Long workers that otherwise log only on completion use it
// to emit a periodic progress line, so a stalled run is visible (the line stops
// advancing) instead of going silent for hours. report runs concurrently with the
// work, so any counters it reads must be safe for concurrent access (e.g.
// atomic). stop halts the ticker and the goroutine; call it with defer.
func Heartbeat(interval time.Duration, report func()) (stop func()) {
	ticker := time.NewTicker(interval)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				report()
			case <-done:
				return
			}
		}
	}()
	return func() {
		ticker.Stop()
		close(done)
	}
}
