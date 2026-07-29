package ghost

import "testing"

// silentApp is an application well past the `applied` threshold of 21 days.
func silentApp(jobID, userID int64) Application {
	return Application{
		JobID:          jobID,
		UserID:         userID,
		Stage:          "applied",
		LastActivityAt: now.AddDate(0, 0, -30),
	}
}

// maturedReport is a report whose stated apply date has cleared the same threshold.
func maturedReport(jobID, userID int64) Report {
	return Report{JobID: jobID, UserID: userID, AppliedOn: now.AddDate(0, 0, -30)}
}

func TestAggregate_SilentApplicationCounts(t *testing.T) {
	got := Aggregate(now, []Application{silentApp(1, 10)}, nil)

	ev, ok := got[1]
	if !ok {
		t.Fatalf("job 1 missing from %v", got)
	}
	if ev.SilentApplications != 1 || ev.Contributors != 1 {
		t.Errorf("evidence = %+v, want 1 silent application from 1 contributor", ev)
	}
}

func TestAggregate_ApplicationInsideItsThresholdIsNotEvidence(t *testing.T) {
	app := silentApp(1, 10)
	app.LastActivityAt = now.AddDate(0, 0, -10) // inside the 21-day `applied` threshold

	if got := Aggregate(now, []Application{app}, nil); len(got) != 0 {
		t.Errorf("aggregate = %v, want empty — the application is still answering promptly", got)
	}
}

// A settled application awaits no reply, so counting its silence would
// manufacture an alarm about a closed matter.
func TestAggregate_TerminalStageIsNotEvidence(t *testing.T) {
	for _, stage := range []string{"rejected", "accepted", "withdrawn"} {
		app := silentApp(1, 10)
		app.Stage = stage

		if got := Aggregate(now, []Application{app}, nil); len(got) != 0 {
			t.Errorf("stage %q: aggregate = %v, want empty", stage, got)
		}
	}
}

// Mail the matcher believes belongs to this application contradicts the claim
// that nobody replied. A question is not a fact, and only facts are evidence.
func TestAggregate_PendingSuggestionIsNotEvidence(t *testing.T) {
	app := silentApp(1, 10)
	app.HasPendingSuggestion = true

	if got := Aggregate(now, []Application{app}, nil); len(got) != 0 {
		t.Errorf("aggregate = %v, want empty — unconfirmed mail makes the silence a question", got)
	}
}

// An unset stage is still an application, judged against `applied`.
func TestAggregate_UnsetStageIsJudgedAsApplied(t *testing.T) {
	app := silentApp(1, 10)
	app.Stage = ""

	if got := Aggregate(now, []Application{app}, nil); len(got) != 1 {
		t.Errorf("aggregate = %v, want the application counted", got)
	}
}

func TestAggregate_OnePersonOnBothChannelsIsOneContributor(t *testing.T) {
	got := Aggregate(now, []Application{silentApp(1, 10)}, []Report{maturedReport(1, 10)})

	ev := got[1]
	if ev.Contributors != 1 {
		t.Errorf("contributors = %d, want 1 — the same person arrived twice", ev.Contributors)
	}
	if ev.SilentApplications != 1 || ev.Reports != 1 {
		t.Errorf("evidence = %+v, want both channels recorded", ev)
	}
}

func TestAggregate_TwoPeopleOnDifferentChannelsAreTwoContributors(t *testing.T) {
	got := Aggregate(now, []Application{silentApp(1, 10)}, []Report{maturedReport(1, 11)})

	if ev := got[1]; ev.Contributors != 2 {
		t.Errorf("contributors = %d, want 2", ev.Contributors)
	}
}

// A claim contributes only once the same span has elapsed that the tracking
// board tolerates before calling an application silent.
func TestAggregate_ReportMaturesOnlyAfterTheAppliedThreshold(t *testing.T) {
	fresh := maturedReport(1, 10)
	fresh.AppliedOn = now.AddDate(0, 0, -5)

	if got := Aggregate(now, nil, []Report{fresh}); len(got) != 0 {
		t.Errorf("aggregate = %v, want empty — the claim is 5 days old", got)
	}
}

func TestAggregate_MapHoldsOnlyJobsWithEvidence(t *testing.T) {
	quiet := silentApp(2, 10)
	quiet.LastActivityAt = now // job 2 has an application, but no silence

	got := Aggregate(now, []Application{silentApp(1, 10), quiet}, nil)

	if _, ok := got[2]; ok {
		t.Errorf("aggregate = %v, want job 2 absent — sparse by construction", got)
	}
	if len(got) != 1 {
		t.Errorf("aggregate = %v, want exactly job 1", got)
	}
}

func TestAggregate_SeparatesJobs(t *testing.T) {
	got := Aggregate(now, []Application{silentApp(1, 10), silentApp(2, 11)}, nil)

	if len(got) != 2 {
		t.Fatalf("aggregate = %v, want both jobs", got)
	}
	for jobID, ev := range got {
		if ev.Contributors != 1 {
			t.Errorf("job %d: contributors = %d, want 1 — evidence must not leak between jobs", jobID, ev.Contributors)
		}
	}
}

// The maturity rule and the silence ladder must read the same number. Pinning
// them to each other here means a change to the ladder cannot silently leave the
// report channel judging by a stale threshold.
func TestAggregate_ReportMaturityTracksTheAppliedThreshold(t *testing.T) {
	threshold, ok := appliedThresholdDays()
	if !ok {
		t.Fatal("the applied stage must carry a silence threshold")
	}

	atThreshold := maturedReport(1, 10)
	atThreshold.AppliedOn = now.AddDate(0, 0, -threshold)
	if got := Aggregate(now, nil, []Report{atThreshold}); len(got) != 0 {
		t.Errorf("aggregate = %v, want empty at exactly the threshold — it is the last tolerated day", got)
	}

	pastThreshold := maturedReport(1, 10)
	pastThreshold.AppliedOn = now.AddDate(0, 0, -(threshold + 1))
	if got := Aggregate(now, nil, []Report{pastThreshold}); len(got) != 1 {
		t.Errorf("aggregate = %v, want the claim counted one day past the threshold", got)
	}
}
