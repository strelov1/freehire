package mcpapp

// researchResults adapts a search_jobs answer to the deep-research search shape.
func researchResults(r SearchJobsResult) ResearchResults {
	hits := make([]ResearchHit, 0, len(r.Jobs))
	for _, j := range r.Jobs {
		hits = append(hits, ResearchHit{ID: j.Slug, Title: researchTitle(j), URL: researchURL(j)})
	}
	return ResearchResults{Results: hits}
}

// researchDocument adapts a get_job answer to the deep-research fetch shape.
func researchDocument(j JobResult) ResearchDocument {
	return ResearchDocument{
		ID:    j.Slug,
		Title: researchTitle(j.JobSummary),
		Text:  j.Description,
		URL:   researchURL(j.JobSummary),
		Metadata: ResearchMetadata{
			Company:    j.Company,
			Location:   j.Location,
			Source:     j.Source,
			PostedAt:   j.PostedAt,
			Reality:    j.Reality,
			GhostLevel: j.GhostLevel,
		},
	}
}

// researchTitle names the employer in the title: a research report lists sources by title,
// and "Senior Go Engineer" alone says nothing about whose role it is.
func researchTitle(j JobSummary) string {
	if j.Company == "" {
		return j.Title
	}
	return j.Title + " — " + j.Company
}

// researchURL is the posting's freehire page, or the apply redirect where this deployment has
// no origin configured — a citation needs SOME link a person can follow.
func researchURL(j JobSummary) string {
	if j.URL != "" {
		return j.URL
	}
	return j.ApplyURL
}
