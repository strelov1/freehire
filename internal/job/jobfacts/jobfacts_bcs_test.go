package jobfacts

import "testing"

// TestEmploymentTypeBCS pins the Croatian, Serbian and Bosnian contract phrases, cut from
// live postings placed in hr/rs/ba (talentlyft, eures, zohorecruit, smartrecruiters),
// measured 2026-10-29.
func TestEmploymentTypeBCS(t *testing.T) {
	cases := []struct{ name, title, desc, want string }{
		// The indefinite contract is the permanent one.
		{"neodređeno", "Prodavač (m/ž)", "Što nudimo: ugovor na neodređeno s probnim rokom, prednosti i pogodnosti zaposlenja.", "full_time"},
		{"ugovor o radu", "Vozač", "Nudimo: ugovor o radu na neodređeno vrijeme, uz probni rad 6 mjeseci.", "full_time"},
		{"puno radno vreme", "Inženjer", "Mesto rada: Novi Sad. Radni odnos: puno radno vreme.", "full_time"},
		{"stalni ugovor", "Kuhar", "Što nudimo - stalni ugovori na puno radno vrijeme - plaće od 3.000 EUR bruto.", "full_time"},
		// The fixed-term and the work contract are contract, and outrank a promised
		// permanent one.
		{"određeno", "Konobar", "Ugovor o radu na određeno vrijeme, uz mogućnost nastavka suradnje.", "contract"},
		{"određeno then neodređeno", "Skladištar", "Ugovor na određeno s mogućnošću zapošljavanja na neodređeno vrijeme.", "contract"},
		{"full time fixed term", "Referent", "Ugovor o radu na puno radno vrijeme (40 sati tjedno), na određeno vrijeme u trajanju od 12 mjeseci.", "contract"},
		{"ugovor o delu", "Dizajner", "Angažman po ugovoru o delu. Tražimo: ugovor o delu za projekat.", "contract"},
		{"no diacritics", "Vozac", "Nudimo ugovor na odredjeno vreme.", "contract"},
		// Part-time.
		{"nepuno", "Prodavač", "Ugovor na nepuno radno vrijeme, uz rad u smjenama.", "part_time"},
		// A trainee is an internship, by the title or the internship's own phrase.
		{"pripravnik title", "Programer/ka pripravnik/ica", "Ugovor na određeno vrijeme.", "internship"},
		{"praksa title", "Studentska praksa", "Tražimo studente.", "internship"},
		// Both offered states neither; a parent's shorter hours, good practice and an
		// eligibility clause are not the posting's contract.
		{"either hours", "Prodavač", "Mogućnost rada na puno ili nepuno radno vrijeme.", ""},
		{"either slash", "Bankar u poslovnici", "Vrsta ugovora: na određeno vrijeme / neodređeno vrijeme.", ""},
		{"civil-service staž", "Viši informatičar", "Za radna mjesta pripravnika primaju se osobe s radnim iskustvom kraćim od vremena određenog za pripravnički staž.", ""},
		{"either contract", "Stipendija", "Uvjet: da nisu zaposleni na određeno ili na neodređeno vrijeme.", ""},
		{"parent perk", "Analitičar", "Obiteljski benefiti: skraćeno radno vrijeme nakon povratka s porodiljnog, pola radnog vremena za nove majke.", ""},
		{"good practice", "Analitičar", "Primjena dobre prakse i mentorstvo pripravnika.", ""},
	}
	for _, c := range cases {
		if got := EmploymentType(c.title, c.desc); got != c.want {
			t.Errorf("%s: EmploymentType(%q, %q) = %q, want %q", c.name, c.title, c.desc, got, c.want)
		}
	}
}
