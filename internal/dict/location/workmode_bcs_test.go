package location

import "testing"

// TestBCSWorkMode pins the Croatian, Serbian and Bosnian arrangement prose. The sentences
// are cut from live postings placed in hr/rs/ba (talentlyft, eures, zohorecruit,
// bamboohr, smartrecruiters), measured 2026-10-29.
func TestBCSWorkMode(t *testing.T) {
	cases := []struct {
		name, desc, want string
	}{
		// The place of work is the home, or the totality is stated.
		{"mjesto rada", "<p>Mjesto rada: rad od kuće (cijela BiH)</p><p>Opis tvog budućeg posla</p>", "remote"},
		{"homeoffice", "Poslodavac: Raditi.eu Mjesto rada: Rad od kuće (Homeoffice) Tip zaposlenja: rad na određeno", "remote"},
		{"fully remote", "Potpuno udaljeni rad (rad od kuće), fleksibilan dogovor oko smjena.", "remote"},
		{"100% remote", "Lokacija: 100% remote. Karijera i razvoj.", "remote"},
		{"option of fully remote", "Nudimo mogućnost potpunog remote rada uz veliku slobodu u radu.", "remote"},
		// Home working listed as a benefit, or a split week, is hybrid.
		{"hibridni rad", "<li>Što nudimo: hibridni rad i fleksibilno radno vrijeme</li>", "hybrid"},
		{"ekavian", "Fleksibilno radno vreme, mogućnost hibridnog rada.", "hybrid"},
		{"benefit", "Fleksibilnost, mogućnost rada od kuće, klizno radno vrijeme.", "hybrid"},
		{"occasional", "Mogućnost povremenog rada od kuće nakon probnog roka.", "hybrid"},
		{"na daljinu", "Fleksibilno radno vrijeme i mogućnost rada na daljinu.", "hybrid"},
		{"combined", "Fleksibilan model rada (kombinacija rada od kuće i rada iz ureda).", "hybrid"},
		{"either", "Rad od kuće / rad iz kancelarije - u zavisnosti od projekta.", "hybrid"},
		{"office then remote", "Radni model: prvih 9 tjedana iz ureda (trening), nakon toga moguć remote rad.", "hybrid"},
		{"no diacritics", "Mjesto rada: Zagreb (hibridno).", "hybrid"},
		// Onsite is stated, or home working refused.
		{"iz ureda", "Mjesto rada: Zagreb - rad iz ureda. Tražimo energične osobe.", "onsite"},
		{"iz kancelarije", "Opis posla: rad iz kancelarije u Kragujevcu, rad u svim smenama.", "onsite"},
		{"refused", "Spremnost na rad iz kancelarije na Novom Beogradu, nije rad od kuće.", "onsite"},
		{"weekdays in office", "Rad se odvija iz ureda u Zagrebu, remote dežurstvo vikendom.", "onsite"},
		// A negated offer, weekend on-call duty, a remote-controlled crane and remote
		// support state no arrangement.
		{"not possible", "Nažalost, nije moguć rad od kuće.", "onsite"},
		{"negated", "Za ovu poziciju ne nudimo hibridni rad.", ""},
		{"on-call", "Popodnevna smjena: 15:00 – 22:00. Rad se odvija od kuće (remote dežurstvo) vikendom", ""},
		{"crane", "Daljinski upravljati STS dizalicom za utovar i istovar kontejnera.", ""},
		{"remote support", "Rješavanje tehničkih incidenta i korisničkih zahtjeva (remote i lokalno).", ""},
		{"office coffee", "Kavu i slatkiše u uredu – jer dobre ideje traže dobru kavu.", ""},
	}
	for _, c := range cases {
		if got := WorkModeFromDescription(c.desc); got != c.want {
			t.Errorf("%s: WorkModeFromDescription(%q) = %q, want %q", c.name, c.desc, got, c.want)
		}
	}
}
