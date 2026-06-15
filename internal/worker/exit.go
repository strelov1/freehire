// Package worker holds the shared bootstrap and run-outcome plumbing for the
// standalone run-once-and-exit cron workers under cmd/. It centralizes config +
// pool + signal-bound context setup and the convention that maps a run's failure
// counts to a process exit code, so a degraded run is visible to cron.
package worker

// ExitCode maps a worker run's failure tallies to a process exit code: 0 when the
// run completed cleanly, 1 when it finished with any per-item failures or
// dead-lettered items. A non-zero code lets cron alert on a partially-failed run
// that would otherwise look successful.
func ExitCode(failed, deadLettered int) int {
	if failed > 0 || deadLettered > 0 {
		return 1
	}
	return 0
}
