package emailnotify

import (
	"context"

	"github.com/strelov1/freehire/internal/platform/worker"
)

// Class groups the outbound-mail workers by what holding them costs, so an
// operator can pause digests and nudges while payment confirmations keep going.
type Class string

const (
	// ClassTransactional is mail a person is waiting for after paying.
	ClassTransactional Class = "transactional"
	// ClassEngagement is what the product tells people about activity they asked
	// to hear about: saved-search digests, unread messages, session reminders.
	ClassEngagement Class = "engagement"
	// ClassLifecycle is the courtesy sequences and nudges.
	ClassLifecycle Class = "lifecycle"
)

// mailKeyAll is the suffix that holds every class at once.
const mailKeyAll = "mail:all"

// workerClass maps a worker binary's name to its class. A binary that builds an
// SES client for mail and is missing here is caught by TestEveryMailWorkerIsClassified.
//
// The API server's own mail (verification codes, password reset, application
// mail) is deliberately absent: no class switch may hold it, and the server is
// not a cron worker that consults the pause keys at all. cmd/broadcast is a
// hand-run send and is not held by a class either.
var workerClass = map[string]Class{
	"pro-welcome-mail": ClassTransactional,

	"notify":            ClassEngagement,
	"message-notify":    ClassEngagement,
	"mentorship-remind": ClassEngagement,

	"onboarding":        ClassLifecycle,
	"nudge":             ClassLifecycle,
	"remind":            ClassLifecycle,
	"limit-nudge-mail":  ClassLifecycle,
	"cv-lifecycle-mail": ClassLifecycle,
}

// ClassOf reports the class of a worker binary, and false for one that sends no
// held mail.
func ClassOf(binary string) (Class, bool) {
	c, ok := workerClass[binary]
	return c, ok
}

// ClassKey is the pause key that holds one class. Presence is the signal; delete
// it to resume.
func ClassKey(c Class) string { return worker.PauseKey("mail:" + string(c)) }

// HoldKeys are the keys, beyond the per-binary and fleet-wide ones, that hold
// this worker: its class and all mail. A binary outside the table has none.
// Pass them to worker.Main for a worker whose whole run is mail.
func HoldKeys(binary string) []string {
	c, ok := ClassOf(binary)
	if !ok {
		return nil
	}
	return []string{ClassKey(c), worker.PauseKey(mailKeyAll)}
}

// EmailHeld reports whether a class hold is withholding this worker's email. A
// multi-channel worker (notify, nudge, remind) asks it when it builds its
// router and leaves the email channel out, so the other channels keep running.
// It has the bypass and the fail-open behaviour of worker.Paused.
func EmailHeld(ctx context.Context, redisURL, binary string) bool {
	keys := HoldKeys(binary)
	return len(keys) > 0 && worker.AnyHeld(ctx, redisURL, keys...)
}
