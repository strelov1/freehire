package jobview

import (
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/outboundurl"
)

func applicationDestination(f job.Fields) string {
	if job.IsDirectApply(f.Source, f.ExternalID) {
		return ""
	}
	if f.ApplicationURL != "" {
		return f.ApplicationURL
	}
	return f.URL
}

func originalPosting(f job.Fields) string {
	if f.ApplicationURL == "" || f.ApplicationURL == f.URL || job.IsDirectApply(f.Source, f.ExternalID) {
		return ""
	}
	return outboundurl.Tag(f.URL)
}
