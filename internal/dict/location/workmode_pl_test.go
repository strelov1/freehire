package location

import "testing"

// TestPolishWorkModeFromDescription pins the Polish arrangement phrases. Polish lists
// home working as a benefit beside an office, which is hybrid; remote needs a stated
// totality. Every case is a sentence from a live Polish posting (erecruiter, traffit,
// nofluffjobs, solidjobs, smartrecruiters, teamtailor, recruitee, eures, whatjobs-pl),
// measured 2026-10-29.
func TestPolishWorkModeFromDescription(t *testing.T) {
	cases := []struct {
		desc, want string
	}{
		// Remote: a stated totality, a labelled line, the template's "w całości".
		{"Charakterystyka oferty: miejsce pracy: 100% zdalnie (odbiór sprzętu w biurze klienta)", "remote"},
		{"Mile widziana znajomość Google Cloud. Praca w pełni zdalna, elastyczne godziny.", "remote"},
		{"Forma współpracy: B2B tryb pracy: 100% zdalnie okres współpracy: 13 miesięcy", "remote"},
		{"Elastyczne godziny pracy (100%) Praca zdalna: w całości Kim jesteśmy?", "remote"},
		{"Lokalizacja: zdalnie (z Polski), z okazjonalnymi wizytami w biurze klienta.", "remote"},
		{"Telemedycyna - praca zdalna z dowolnego miejsca w Polsce", "remote"},
		// Hybrid: the model, the office days, home working as a benefit, a choice.
		{"Umowa o pracę, praca hybrydowa (3 dni w biurze + 2 dni zdalnie), opieka medyczna", "hybrid"},
		{"Tryb pracy: hybrydowy (1 dzień w tygodniu w biurze w Warszawie)", "hybrid"},
		{"Lokalizacja: praca w modelu hybrydowym 6 dni na miesiąc z biura w Krakowie", "hybrid"},
		{"Elastyczne godziny pracy (100%) Praca zdalna: hybrydowo Pakiet medyczny", "hybrid"},
		{"Praca zdalna: możliwa częściowo. Kim jesteśmy?", "hybrid"},
		{"Oferujemy: atrakcyjne wynagrodzenie. Możliwość pracy zdalnej. Elastyczne godziny.", "hybrid"},
		{"📍 Warszawa / hybrydowo (1 dzień w tygodniu z biura)", "hybrid"},
		{"Elastyczne grafiki, możliwość pracy stacjonarnej, zdalnej lub hybrydowej.", "hybrid"},
		{"Model pracy: 80% zdalnie, raz w tygodniu spotkanie zespołu.", "hybrid"},
		// Onsite: a stated office model, the template's "brak".
		{"Elastyczne godziny pracy (100%) Praca zdalna: brak Pakiet medyczny", "onsite"},
		{"Lokalizacja i tryb pracy: ul. Spokojna 2 w Lublinie, praca stacjonarna", "onsite"},
		{"Lokalizacja: Al. Niepodległości 18 Tryb pracy: stacjonarnie Języki: polski", "onsite"},
		{"Nie ma możliwości pracy zdalnej na tym stanowisku.", "onsite"},
		// The hiring process, a tutoring lesson, a cloud and a negation are not the job.
		{"Rekrutacja: 100% zdalna. Dlaczego ta rola?", ""},
		{"Proces rekrutacyjny (100% zdalnie): krótka rozmowa telefoniczna.", ""},
		{"Lekcje online lub stacjonarnie – ty decydujesz.", ""},
		{"Projekt skupia się na rozbudowie nowoczesnej platformy w modelu hybrydowym oraz multi-cloud.", ""},
		{"Prowadzimy sprzedaż stacjonarną i online.", ""},
		{"Nie oferujemy pracy hybrydowej.", ""},
		{"Możliwość pracy zdalnej 20 dni w roku.", ""},
	}
	for _, c := range cases {
		if got := WorkModeFromDescription(c.desc); got != c.want {
			t.Errorf("WorkModeFromDescription(%q) = %q, want %q", c.desc, got, c.want)
		}
	}
}
