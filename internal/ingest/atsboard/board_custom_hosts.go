package atsboard

import (
	"net/url"
	"regexp"
	"strings"
)

var fordRecruitingSite = regexp.MustCompile(`^/(?i:[a-z_-]+/sites)/CX_1(?:/|$)`)

// recognizeCustomCareerHost covers verified employer domains whose job pages
// may be unreachable even while the corresponding board's feed is available.
func recognizeCustomCareerHost(u *url.URL) (source, board, canonical string, ok bool) {
	if u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return "", "", "", false
	}
	switch host := strings.ToLower(u.Hostname()); host {
	case "careers.arnoldclark.com":
		// Firefish reads this host's RSS even after a posting page returns 410.
		return "firefish", host, "https://" + host, true
	case "apply.ford.com":
		// Ford's branded entry point serves Oracle's CX_1 site from this pod.
		if fordRecruitingSite.MatchString(u.Path) {
			return "oracle", "efds.fa.em5.oraclecloud.com/CX_1",
				"https://efds.fa.em5.oraclecloud.com/hcmUI/CandidateExperience/en/sites/CX_1", true
		}
	}
	return "", "", "", false
}
