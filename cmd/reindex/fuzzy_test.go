package main

import "slices"

const (
	// A prod-like body: the localized block that differs between cities is a small fraction of it.
	bodyA = "Build and operate the payments platform with Golang, TypeScript and Postgres. " +
		"Design resilient APIs, review pull requests, instrument services with metrics and traces, " +
		"run blameless incident retrospectives, mentor junior engineers, shape the quarterly roadmap, " +
		"migrate legacy batch pipelines towards streaming, tighten database indexes and query plans, " +
		"automate deployments through continuous delivery, write architectural decision records, " +
		"improve developer tooling, reduce build times, keep documentation accurate as the platform evolves. " +
		"Our workloads run containerised across several availability zones with asynchronous messaging, " +
		"idempotent consumers and careful backpressure handling whenever traffic surges unexpectedly. " +
		"We practise trunk based development, keep feature flags short lived, prefer boring technology " +
		"where reliability matters more than novelty, and rotate engineers through customer support weeks " +
		"so operational pain reaches whoever can remove it. Compensation reviews happen twice yearly, " +
		"equipment budgets renew annually, conference attendance receives support, remote colleagues gather " +
		"quarterly for planning workshops, and internal mobility between squads remains actively encouraged."
	// A different role at the same company: shares the employer boilerplate, not the specialty.
	bodyB = "Advise retail clients on supply chain strategy, run stakeholder workshops, " +
		"prepare board level presentations, build financial models, benchmark competitors, " +
		"lead procurement negotiations, design operating models, coach client teams through change, " +
		"quantify savings opportunities, prioritise transformation initiatives, report to steering committees."
)

// sortedIDs makes the judged set comparable: it is built by iterating a map, so its order is
// not stable across runs.
func sortedIDs(ids []int64) []int64 {
	out := append([]int64(nil), ids...)
	slices.Sort(out)
	return out
}
