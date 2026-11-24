package jobview

import (
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/outboundurl"
	"github.com/strelov1/freehire/internal/platform/db"
)

// RowDestination is where the click redirect sends a visitor for a stored job: the
// application destination, "" when the job takes applications on freehire itself.
func RowDestination(r db.Job) string {
	return applicationDestination(job.Fields{Source: r.Source, ExternalID: r.ExternalID, URL: r.URL, ApplicationURL: r.ApplicationUrl})
}

func applyURL(f job.Fields) string {
	// The click redirect refuses a private job (its owner is not known there), so the owner's own link stays direct.
	if f.Private || applicationDestination(f) == "" {
		return ""
	}
	return outboundurl.ApplyURL(f.PublicSlug, outboundurl.SrcWeb)
}

// ApplyURLFor is the click redirect for a surface other than the web, which ApplyURL already
// serves: "" for a job with no employer destination, exactly as ApplyURL is.
func (j Job) ApplyURLFor(src outboundurl.Src) string {
	if j.ApplyURL == "" {
		return ""
	}
	return outboundurl.ApplyURL(j.PublicSlug, src)
}

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
