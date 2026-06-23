package main

import (
	"context"
	"fmt"
	"regexp"
)

// paycomProber validates a Paycom portal "<clientkey>" by reading the per-portal session JWT
// from its SSR page, then counting open postings via the regional Mantle API. Paycom exposes
// the employer name at /api/ats/company-name. See internal/sources/paycom.go for the contract.
type paycomProber struct{}

var (
	paycomProbeJWT  = regexp.MustCompile(`"sessionJWT":"([^"]+)"`)
	paycomProbeHost = regexp.MustCompile(`portal-applicant-tracking\.[a-z0-9-]+\.paycomonline\.net`)
)

func (paycomProber) probe(ctx context.Context, c httpClient, clientkey string) (string, int, error) {
	page, err := c.GetText(ctx, fmt.Sprintf("https://www.paycomonline.net/v4/ats/web.php/portal/%s/jobs/1", clientkey))
	if err != nil {
		return "", 0, nil
	}
	jm := paycomProbeJWT.FindStringSubmatch(page)
	host := paycomProbeHost.FindString(page)
	if jm == nil || host == "" {
		return "", 0, nil
	}
	mantle := "https://" + host
	auth := map[string]string{"Authorization": jm[1], "Locale": "en-US"}

	body := map[string]any{"skip": 0, "take": 1, "filtersForQuery": map[string]any{
		"distanceFrom": 0, "workEnvironments": []any{}, "positionTypes": []any{}, "educationLevels": []any{},
		"categories": []any{}, "travelTypes": []any{}, "shiftTypes": []any{}, "otherFilters": []any{},
		"keywordSearchText": "", "location": "", "sortOption": "",
	}}
	var sr struct {
		Count int `json:"jobPostingPreviewsCount"`
	}
	if err := c.PostJSONWithHeaders(ctx, mantle+"/api/ats/job-posting-previews/search", auth, body, &sr); err != nil {
		return "", 0, nil
	}
	if sr.Count == 0 {
		return "", 0, nil
	}
	var cn struct {
		CompanyName string `json:"companyName"`
	}
	_ = c.GetJSONWithHeaders(ctx, mantle+"/api/ats/company-name", auth, &cn)
	return cn.CompanyName, sr.Count, nil
}
