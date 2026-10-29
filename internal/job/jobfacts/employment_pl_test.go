package jobfacts

import "testing"

// TestEmploymentTypePolish pins the Polish contract words. Every case is a line from a
// live Polish posting (erecruiter, traffit, nofluffjobs, solidjobs, smartrecruiters,
// teamtailor, whatjobs-pl, eures), measured 2026-10-29.
func TestEmploymentTypePolish(t *testing.T) {
	cases := []struct {
		title, desc, want string
	}{
		// The employment contract is full time unless a share is stated.
		{"Specjalista ds. zakupów", "Oferujemy: stabilne zatrudnienie w oparciu o umowę o pracę.", "full_time"},
		{"Pracownik/Pracowniczka Sklepu (pełny etat)", "Twoje zadania: dbałość o ekspozycję.", "full_time"},
		{"Senior Consultant", "Fully remote from Poland, UoP.", "full_time"},
		// A part-time share, and a side job.
		{"Analityk", "Rodzaj umowy: umowa o pracę (1/2 etatu).", "part_time"},
		{"Pracownik/Pracowniczka Sklepu - 75% etatu", "", "part_time"},
		{"Doradca Klienta – niepełny etat (m/k)", "Lovisa to firma zajmująca się sprzedażą.", "part_time"},
		{"PRACA DODATKOWA Ochrona sklepu w niedziele", "", "part_time"},
		// A choice that includes full time is not part time.
		{"Księgowa", "Zatrudnienie w ramach umowy o pracę - na pełen etat lub na 3/4 etatu.", "full_time"},
		// B2B as the contract, the civil-law contracts, a replacement contract.
		{"Senior Data Scientist", "Stawka: 190-230 zł/h netto + VAT na B2B.", "contract"},
		{"Platform Engineer", "Jakie warunki otrzymasz? 23k–26k PLN netto/m (B2B)", "contract"},
		{"Developer", "Salary: 1000-1300 PLN/day on B2B. Work model: hybrid.", "contract"},
		{"Analityk", "Forma współpracy: B2B z ITFS, stawka do 130 PLN/h.", "contract"},
		{"Kasjer", "Oferujemy umowę zlecenie i elastyczny grafik.", "contract"},
		{"Sprzedawca", "Umowa na zastępstwo, 1/1 etatu.", "contract"},
		// The internship: the title's words, a paid internship, the internship contract.
		{"Stażystka/Stażysta w obszarze ryzyka kredytowego", "", "internship"},
		{"Praktykant/ka (branża mostowa)", "", "internship"},
		{"Asystent", "Oferujemy płatny staż w oparciu o umowę zlecenie.", "internship"},
		// "Staż pracy" is work experience, and B2B a business model.
		{"Kierownik sklepu", "Nagroda za staż pracy, karta sportowa.", ""},
		{"Handlowiec", "Sprzedaż B2B do klientów z branży logistycznej.", ""},
		{"Key Account Manager", "Znajomość modeli sprzedażowych i kontraktowych B2B.", ""},
	}
	for _, c := range cases {
		if got := EmploymentType(c.title, c.desc); got != c.want {
			t.Errorf("EmploymentType(%q, %q) = %q, want %q", c.title, c.desc, got, c.want)
		}
	}
}
