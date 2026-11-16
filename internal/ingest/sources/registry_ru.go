package sources

// ruCompanySources are the RU-domestic single-company adapters (see All).
func ruCompanySources(c HTTPClient) []Source {
	// Alfa-Bank, Positive Technologies and Tochka Bank serve certificates issued by the
	// Russian Trusted Root CA, which no standard trust store carries, so they fail the TLS
	// handshake on the shared client. They share one client that trusts that extra root —
	// see NewRussianTrustedRootClient for why the scope is these adapters and not the fleet.
	russianRoot := NewRussianTrustedRootClient()
	return []Source{
		// RU-domestic single-company adapters (boardless, except Yandex which selects
		// host+language by board).
		NewYandex(c),
		// Yandex Crowd: gig/support catalogue at crowd.yandex.ru, one JSON blob per landing page.
		NewYandexCrowd(c),
		NewOzon(c),
		NewAvito(c),
		// Kaspi.kz (Kazakhstan): the same sitemap + JobPosting ld+json shape as Avito.
		NewKaspi(c),
		// Halyk Bank (Kazakhstan): one HTML listing, then a page per vacancy. The path
		// number is a list position, not an id — see halyk.go.
		NewHalyk(c),
		NewX5(c),
		NewRWB(c),
		NewSber(c),
		// Uzum (Uzbekistan): sitemap + per-vacancy RSC flight; see uzum.go.
		NewUzum(c),
		NewAlfaBank(russianRoot),
		// Positive Technologies and Tochka Bank: their postings exist only on their own
		// Next.js careers sites, read from the server-rendered pages.
		NewPtsecurity(russianRoot),
		NewTochka(russianRoot),
		NewLamoda(c),
		NewKuper(c),
		NewAviasales(c),
		NewDodo(c),
		NewDomclick(c),
		NewMtslink(c),
		// www.tbank.ru serves a certificate chaining to the Russian Trusted Root CA too, and its
		// sitemaps are slow to serve, hence the longer timeout (see tbankTimeout).
		NewTBank(newRussianTrustedRootClient(tbankTimeout)),
		NewMTS(c),
		// MegaFon walks its listing city by city (the unfiltered feed's order is unstable and
		// deterministically hides some postings) and hydrates bodies; see megafon.go.
		NewMegafon(c),
		NewRostelecom(c),
		NewVK(c),
		NewTwoGIS(c),
		// Kaspersky: no public API; the listing's RSC flight carries the structured fields
		// and the posting page the body. See kaspersky.go.
		NewKaspersky(c),
		// Kontur: one listing page plus a JobPosting ld+json per vacancy. Proxied — see proxy.go.
		NewKontur(c),
		// Magnit Tech: the retail group's IT arm, a keyless JSON API (list → detail).
		NewMagnit(c),
		NewSelectel(c),
	}
}
