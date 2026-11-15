package handler

import (
	"context"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/isoweek"
)

// skillHistoryReader is the slice of the DB the market-pulse read needs (*db.Queries
// satisfies it), kept narrow so the handler is unit-testable without a database.
type skillHistoryReader interface {
	ListInsightsSkillHistory(ctx context.Context, skills []string) ([]db.InsightsSkillHistory, error)
}

// marketPulseHandlers serves the signed-in user's personal skill-demand trend: their profile
// skills joined against the weekly insights_skill_history snapshots cmd/rollup-stats writes.
type marketPulseHandlers struct {
	userProfile *userprofile.Service
	history     skillHistoryReader
}

func newMarketPulseHandlers(userProfile *userprofile.Service, history skillHistoryReader) *marketPulseHandlers {
	return &marketPulseHandlers{userProfile: userProfile, history: history}
}

func (h *marketPulseHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// Cookie-only like the other personal /me/* browser surfaces: a leaked API key must not
	// read a candidate's skill trend.
	api.Get("/me/market-pulse", mw.cookie, contract)
}

// GetMarketPulse returns the caller's profile skills' demand trend. No profile, or no
// skills, gives an empty data array: nothing to plot yet, not a failure.
func (h *marketPulseHandlers) GetMarketPulse(ctx context.Context) (*apigen.GetMarketPulseOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	profile, err := h.userProfile.Get(ctx, userID)
	if err != nil && !errors.Is(err, userprofile.ErrNotFound) {
		return nil, err
	}
	meta := apigen.MarketPulseMeta{WeekStart: isoweek.Start(time.Now().UTC()).Format("2006-01-02")}
	if len(profile.Skills) == 0 {
		return &apigen.GetMarketPulseOK{Data: []apigen.SkillPulse{}, Meta: meta}, nil
	}

	rows, err := h.history.ListInsightsSkillHistory(ctx, profile.Skills)
	if err != nil {
		return nil, err
	}
	return &apigen.GetMarketPulseOK{Data: buildSkillPulses(profile.Skills, rows), Meta: meta}, nil
}

// buildSkillPulses groups the caller's skill-history rows by skill (keeping the caller's
// skill order) and derives each skill's current count, percent change and chronological
// series. Rows arrive newest-first per skill (ListInsightsSkillHistory's ORDER BY skill,
// week_start DESC); a skill with no rows is omitted rather than reported with a made-up count.
func buildSkillPulses(skills []string, rows []db.InsightsSkillHistory) []apigen.SkillPulse {
	bySkill := make(map[string][]db.InsightsSkillHistory, len(skills))
	for _, r := range rows {
		bySkill[r.Skill] = append(bySkill[r.Skill], r)
	}

	out := make([]apigen.SkillPulse, 0, len(skills))
	for _, skill := range skills {
		newestFirst := bySkill[skill]
		if len(newestFirst) == 0 {
			continue
		}

		series := make([]apigen.SkillPulsePoint, len(newestFirst))
		for i, r := range newestFirst {
			series[len(newestFirst)-1-i] = apigen.SkillPulsePoint{
				WeekStart: r.WeekStart.Time.Format("2006-01-02"),
				OpenCount: int64(r.OpenCount),
			}
		}

		var changePct apigen.NilFloat64
		changePct.SetToNull()
		if len(series) >= 2 && series[0].OpenCount > 0 {
			earliest, latest := series[0].OpenCount, series[len(series)-1].OpenCount
			pct := (float64(latest) - float64(earliest)) / float64(earliest) * 100
			changePct.SetTo(pct)
		}

		out = append(out, apigen.SkillPulse{
			Skill:     skill,
			OpenCount: int64(newestFirst[0].OpenCount),
			ChangePct: changePct,
			Series:    series,
		})
	}
	return out
}
