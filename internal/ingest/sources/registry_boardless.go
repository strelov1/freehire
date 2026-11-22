package sources

// boardlessJobBoardSources lists public national and specialist job boards with no tenant key.
func boardlessJobBoardSources(c HTTPClient) []Source {
	// Unstop returns several megabytes per page and can exceed the shared client's timeout.
	var unstopHTTP JSONGetter
	if c != nil {
		unstopHTTP = pacedUnstopGetter(NewLongTimeoutClient())
	}
	return []Source{
		// Unstop: Indian student and early-career platform, jobs and internships. The open
		// catalogue is one keyless public search feed per kind with bodies inline, paced to its
		// robots.txt Crawl-delay (pacedUnstopGetter). See unstop.go.
		NewUnstop(unstopHTTP),
		// ICTJobs: Switzerland's IT-only job board. post-sitemap.xml names the whole catalogue;
		// posting pages carry a schema.org JobPosting. Paced, see ictjobs.go.
		NewICTJobs(c),
		// Saramin: South Korea's largest general job board, every trade. The site's own category
		// listing names the whole catalogue; each new posting's body is read from the iframe its page
		// embeds. Paced, and the edge wants the browser-compatible User-Agent. See saramin.go.
		compatibleUASource(c, NewSaramin),
		// VietnamWorks: Vietnam's largest job board, listed from the search API its own frontend
		// calls and hydrating each new posting's body from the flight stream its page inlines.
		// Served over plain HTTP. See vietnamworks.go.
		NewVietnamworks(c),
		// eFinancialCareers: the global finance and fintech job board, read from the JSON search
		// API behind its own frontend (rows carry the whole body), the whole catalogue every run,
		// cut by country and sector under the search's 10,000-result cap. Plain HTTP from the
		// crawl host. See efinancialcareers.go.
		NewEFinancialCareers(c),
		// Authentic Jobs: a design and developer job board on WordPress / WP Job Manager,
		// boardless. robots.txt bars query strings and sets a 3 s crawl delay, so the catalogue is
		// read through the plugin's query-free listing endpoint and the REST item route, paced.
		// See authenticjobs.go.
		NewAuthenticJobs(c),
		// JobKorea (jobkorea.co.kr): South Korea's general job board, read whole from the sitemaps
		// robots.txt names, each new posting's page and body frame read plainly (no bot wall).
		// See jobkorea.go.
		NewJobKorea(c),
		// HelloWork (France): a window of the catalogue read from the site's clean-URL landing pages,
		// because robots.txt disallows every query string and the sitemap host answers 403; each new
		// posting's body is read from its page. Plain client, paced to one request a second. See
		// hellowork.go.
		NewHelloWork(c),
		// karriere.at: Austria's largest general job board, enumerated from its jobs sitemap, each new
		// posting read from its page's schema.org JobPosting over a plain client. See karriereat.go.
		NewKarriereAt(c, c),
	}
}
