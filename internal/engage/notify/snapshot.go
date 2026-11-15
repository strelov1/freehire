package notify

import (
	"encoding/json"
	"fmt"
)

// SnapshotJob is one entry of user_notifications.jobs: the {title, company, slug}
// shape migration 0091 defined and /my/notifications/[id]/jobs renders.
//
// It lives here because all three notification engines write that column and one
// page reads it.
type SnapshotJob struct {
	Title   string `json:"title"`
	Company string `json:"company"`
	Slug    string `json:"slug"`
}

// JobsSnapshot marshals the job list a multi-job notification records for its own
// page. It applies no bound: the caller's batch is capped before it is sent, so the
// delivery and the record cannot disagree.
func JobsSnapshot(jobs []SnapshotJob) json.RawMessage {
	raw, err := json.Marshal(jobs)
	if err != nil {
		// Every field is a plain string; Marshal only fails on unsupported types
		// (channels, funcs, cyclic refs), none of which SnapshotJob has.
		panic(fmt.Sprintf("notify: marshal jobs snapshot: %v", err))
	}
	return raw
}
