package atsboard

import "testing"

// Vendor hosts and paths the LinkedIn employer-website discovery onboarded as boards on
// 2026-11-17. Each was probed: none is a tenant.
func TestRecognizeVendorHostsFromEmployerCareersPagesAreNotBoards(t *testing.T) {
	for _, raw := range []string{
		// Aon's careers page loads its cookie banner from here; it answers 404.
		"https://cookie-policy-scripts.icims.com/cookie-policy.js",
		// Tellent (Recruitee's owner) loads analytics from here; the offers API answers nothing.
		"https://tagging-server.recruitee.com/gtm.js",
		// Recruitee's fallback host: it redirects to recruitee.com/careers_not_hosted.
		"https://career.recruitee.com/o/engineer",
		// SmartRecruiters' candidate portal, linked from Tieto's careers page; 0 postings.
		"https://jobs.smartrecruiters.com/my-applications",
		"https://careers.smartrecruiters.com/my-applications/",
	} {
		source, board, canonical, ok := Recognize(raw)
		if ok || source != "" || board != "" || canonical != "" {
			t.Errorf("vendor host recognised as employer board: %s -> %q/%q %q ok=%v", raw, source, board, canonical, ok)
		}
	}
	// Neighbouring tenants keep working: qa.recruitee.com is QA Ltd's live board.
	for _, tc := range []struct{ url, source, board string }{
		{"https://qa.recruitee.com/o/business-analyst", "recruitee", "qa"},
		{"https://careers-aon.icims.com/jobs/search", "icims", "aon"},
		{"https://uscareers-medpace.icims.com/jobs/1/engineer/job", "icims", "uscareers-medpace.icims.com"},
		{"https://jobs.smartrecruiters.com/Tieto", "smartrecruiters", "Tieto"},
	} {
		source, board, _, ok := Recognize(tc.url)
		if !ok || source != tc.source || board != tc.board {
			t.Errorf("legitimate tenant overblocked: %s -> %q/%q ok=%v", tc.url, source, board, ok)
		}
	}
}
