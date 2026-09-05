// Command backfill-experience-dates fills experience_employments' structured
// period_start_year/month and period_end_year/month columns (migration 0135) from the
// free-text period_start/period_end columns they are replacing, then exits.
//
// Every write path going forward writes the structured columns directly (see
// internal/candidate/experience); this is the one-off pass for rows written before that
// code shipped. A label that parses (internal/candidate/perioddate.Parse: "2024",
// "October 2018", "2023-09", ...) fills exactly what it says. A label that is empty or
// reads as "not ended" (Present/Current/...) is left unset — that is a real absence, not
// a parse failure, and papering over it with a fabricated date would be worse than
// leaving it blank. Only genuinely garbled text (non-empty, not a present label, and
// still unparseable) falls back to the row's own created_at year: an approximate date
// about a real row reads better to its owner than an empty one. Rare in practice — real
// data is "2024"/"October 2018"-shaped — but a free-text field accepts anything.
//
// Idempotent and safe to stop and resume: SetExperienceEmploymentBackfilledDates fills
// each period boundary independently, only when it is still NULL, so a boundary already
// filled (by this worker or by the ordinary write paths once deployed) is never touched
// again. Also corrects is_current — never back to false, only ever to true — for a row
// whose free-text period_end reads as a present label ("Present", "Current", ...) that
// is_current disagrees with: the pre-migration sort key read that label as "ongoing"
// independently of is_current, so the correction preserves that behavior once the
// free-text column backing it is gone. Needs only DATABASE_URL.
//
//	go run ./cmd/backfill-experience-dates
package main

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/perioddate"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() { worker.Main(run) }

func run() int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	q := db.New(pool)
	rows, err := q.ListExperienceEmploymentDatesForBackfill(ctx)
	if err != nil {
		log.Printf("backfill-experience-dates: list: %v", err)
		return 1
	}
	if len(rows) == 0 {
		log.Print("backfill-experience-dates: nothing to do")
		return 0
	}
	log.Printf("backfill-experience-dates: %d rows to fill", len(rows))

	var filled, fellBack, currentFixed int
	lastLog := time.Now()
	for i, row := range rows {
		start, startFellBack := parseOrFallback(row.PeriodStart, row.CreatedAt.Time)
		end, endFellBack := parseOrFallback(row.PeriodEnd, row.CreatedAt.Time)
		if startFellBack || endFellBack {
			fellBack++
		}
		// The pre-migration sort key read a present-reading period_end as "ongoing"
		// independently of is_current (see period_sort.go, since deleted); a row where the
		// two disagreed needs is_current corrected here, or it silently loses that "ongoing"
		// sort position once the free-text column backing the old check is gone.
		setCurrent := !row.IsCurrent && perioddate.IsPresentLabel(strings.TrimSpace(row.PeriodEnd))
		if setCurrent {
			currentFixed++
		}
		startYear, startMonth := experience.PeriodToColumns(start)
		endYear, endMonth := experience.PeriodToColumns(end)
		n, err := q.SetExperienceEmploymentBackfilledDates(ctx, db.SetExperienceEmploymentBackfilledDatesParams{
			ID:              row.ID,
			PeriodStartYear: startYear, PeriodStartMonth: startMonth,
			PeriodEndYear: endYear, PeriodEndMonth: endMonth,
			SetCurrent: setCurrent,
		})
		if err != nil {
			log.Printf("backfill-experience-dates: row %s after %d filled: %v", row.ID, filled, err)
			return 1
		}
		filled += int(n)

		if time.Since(lastLog) >= time.Minute {
			log.Printf("backfill-experience-dates: progress %d/%d", i+1, len(rows))
			lastLog = time.Now()
		}
		select {
		case <-ctx.Done():
			log.Printf("backfill-experience-dates: cancelled after %d filled, resume by re-running", filled)
			return 1
		default:
		}
	}
	log.Printf("backfill-experience-dates: done, filled=%d (fell back to created_at year for %d unparseable labels, corrected is_current for %d rows)", filled, fellBack, currentFixed)
	return 0
}

// parseOrFallback reads one free-text period label. An empty or present-reading label
// ("Present", "Current", ...) means the period genuinely was not stated — nil, no
// fallback. A label that parses is used as-is. Anything else (non-empty, not a present
// label, still unparseable) falls back to createdAt's year — see the package doc.
// fellBack reports whether that fallback fired, purely for the run's own summary log.
func parseOrFallback(raw string, createdAt time.Time) (d *perioddate.PeriodDate, fellBack bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || perioddate.IsPresentLabel(trimmed) {
		return nil, false
	}
	if parsed, ok := perioddate.Parse(trimmed); ok {
		return parsed, false
	}
	// Sanitize like every other write path does, even though createdAt's year is
	// practically always in range: a fabricated date is only as safe as the same bounds
	// check every candidate-entered or LLM-produced one goes through.
	return perioddate.Sanitize(&perioddate.PeriodDate{Year: createdAt.Year()}), true
}
