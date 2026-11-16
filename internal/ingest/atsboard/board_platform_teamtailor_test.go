package atsboard

import "testing"

func TestRecognizeTeamtailorSharedMediaCDNIsNotAnEmployerBoard(t *testing.T) {
	// Observed in careers.team.blue's CSS backgrounds on 2026-11-16. The media
	// host carries shared assets; it must not onboard a new employer catalogue.
	for _, raw := range []string{
		"https://media.cdn.teamtailor.com/images/s3/teamtailor-production/gallery_picture-v2/image_uploads/4031fbc7-c27f-4928-903e-947f981bc879/original.png",
		"https://media.cdn.teamtailor.com/",
		"https://media.cdn.teamtailor.com/jobs/123-engineer",
	} {
		source, board, canonical, ok := Recognize(raw)
		if ok || source != "" || board != "" || canonical != "" {
			t.Errorf("asset host recognised as employer board: %s -> %q/%q %q ok=%v", raw, source, board, canonical, ok)
		}
	}
	// The exclusion belongs to this literal shared host, not the word "media"
	// across every tenant/platform's namespace.
	for _, tc := range []struct{ url, source, board, canonical string }{
		{"https://media.teamtailor.com/jobs/123-engineer", "teamtailor", "media.teamtailor.com", "https://media.teamtailor.com"},
		{"https://media.recruitee.com/o/software-engineer", "recruitee", "media", "https://media.recruitee.com"},
	} {
		source, board, canonical, ok := Recognize(tc.url)
		if !ok || source != tc.source || board != tc.board || canonical != tc.canonical {
			t.Errorf("legitimate tenant shape overblocked: %s -> %q/%q %q ok=%v", tc.url, source, board, canonical, ok)
		}
	}
}
