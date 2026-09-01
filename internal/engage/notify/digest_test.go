package notify

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

// digestOf builds a Digest of n synthetic jobs, Total matching.
func digestOf(n int) Digest {
	d := Digest{SavedSearchName: "Go", Total: n}
	for i := range n {
		d.Jobs = append(d.Jobs, DigestJob{Title: "Job " + strconv.Itoa(i), Slug: "job-" + strconv.Itoa(i)})
	}
	return d
}

func TestDigestListed_CapsAtListLimit(t *testing.T) {
	got := digestOf(67).Listed()
	if len(got) != ListLimit {
		t.Fatalf("len(Listed()) = %d, want %d", len(got), ListLimit)
	}
	if got[0].Slug != "job-0" || got[ListLimit-1].Slug != "job-"+strconv.Itoa(ListLimit-1) {
		t.Errorf("Listed() = %v…%v, want the first %d in order", got[0].Slug, got[ListLimit-1].Slug, ListLimit)
	}
}

func TestDigestListed_ShortDigestIsWhole(t *testing.T) {
	if got := digestOf(3).Listed(); len(got) != 3 {
		t.Errorf("len(Listed()) = %d, want 3", len(got))
	}
	if got := digestOf(0).Listed(); len(got) != 0 {
		t.Errorf("len(Listed()) = %d, want 0", len(got))
	}
}

// A renderer that appends to Listed()'s result must not scribble over the
// digest's own snapshot, which is recorded separately.
func TestDigestListed_AppendDoesNotOverwriteJobs(t *testing.T) {
	d := digestOf(67)
	appended := append(d.Listed(), DigestJob{Title: "intruder", Slug: "intruder"})
	if d.Jobs[ListLimit].Slug == "intruder" {
		t.Fatalf("appending to Listed() overwrote Jobs[%d]: %+v", ListLimit, d.Jobs[ListLimit])
	}
	if len(appended) != ListLimit+1 {
		t.Errorf("len(appended) = %d, want %d", len(appended), ListLimit+1)
	}
}

func TestBuildDigest_RecordsEveryMatchUnderTheSnapshotCap(t *testing.T) {
	rows := make([]db.GetJobsForDigestRow, 67)
	for i := range rows {
		rows[i] = db.GetJobsForDigestRow{ID: int64(i), Title: "Job " + strconv.Itoa(i), PublicSlug: "job-" + strconv.Itoa(i)}
	}

	d := buildDigest("Go", rows)

	if d.Total != 67 {
		t.Errorf("Total = %d, want 67", d.Total)
	}
	if len(d.Jobs) != 67 {
		t.Errorf("len(Jobs) = %d, want 67 — the snapshot is not the message listing", len(d.Jobs))
	}
	if len(d.Listed()) != ListLimit {
		t.Errorf("len(Listed()) = %d, want %d", len(d.Listed()), ListLimit)
	}
}

// A digest never announces a job it cannot show: deferOverflow holds back
// anything that would not fit before buildDigest sees it.
func TestBuildDigest_TotalMatchesWhatItCarries(t *testing.T) {
	rows := make([]db.GetJobsForDigestRow, 12)
	for i := range rows {
		rows[i] = db.GetJobsForDigestRow{ID: int64(i), Title: "Job " + strconv.Itoa(i)}
	}

	d := buildDigest("Go", rows)

	if d.Total != len(d.Jobs) {
		t.Errorf("Total = %d but carries %d jobs — a digest must not count what it cannot show", d.Total, len(d.Jobs))
	}
}

// The regression this change exists to prevent: the in-app snapshot used to be
// truncated by the email's listing cap.
func TestDigestJobsSnapshot_CarriesEveryRecordedJob(t *testing.T) {
	raw := digestJobsSnapshot(digestOf(67))

	var jobs []SnapshotJob
	if err := json.Unmarshal(raw, &jobs); err != nil {
		t.Fatalf("snapshot did not unmarshal: %v", err)
	}
	if len(jobs) != 67 {
		t.Fatalf("snapshot carries %d jobs, want 67", len(jobs))
	}
}
