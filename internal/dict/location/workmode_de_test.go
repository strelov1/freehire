package location

import "testing"

// German work-arrangement prose. Every positive below is a real sentence (lightly trimmed)
// from the German ATS crawl (coveto, rexx, dvinci, onlyfy, kenjo; 2026-10-25), and every
// negative is a shape the same crawl showed would read backwards.
func TestWorkModeFromDescription_German(t *testing.T) {
	tests := []struct {
		name string
		desc string
		want string
	}{
		// Hybrid — home working offered alongside an office.
		{"homeoffice möglich", "<li>Firmen-PKW inkl. Privatnutzung | 30 Urlaubstage | Homeoffice möglich</li>", "hybrid"},
		{"days of homeoffice", "Wir bieten dir flexible Arbeitszeiten &amp; 2 Tage Homeoffice, 30 Tage Urlaub", "hybrid"},
		{"bis zu 3 tage homeoffice", "Steuerfachangestellter (m/w/d) – bis zu 3 Tage Homeoffice", "hybrid"},
		{"homeoffice nach absprache", "Homeoffice nach Absprache (1 Tag/Woche)", "hybrid"},
		{"homeoffice-möglichkeiten", "flexible Arbeitszeiten &amp; Homeoffice-Möglichkeiten", "hybrid"},
		{"homeoffice-anteil", "flexibles Arbeiten mit bis zu 40%-Homeoffice-Anteil", "hybrid"},
		{"anteiliges homeoffice", "Standort: 84174 Eching &amp; anteiliges Homeoffice", "hybrid"},
		{"teilweise im homeoffice", "die Möglichkeit, teilweise im Homeoffice zu arbeiten", "hybrid"},
		{"mobiles arbeiten days", "bis zu zwei Tage mobiles Arbeiten pro Woche", "hybrid"},
		{"mobiles arbeiten percent", "Mobiles Arbeiten (bis zu 30% pro Monat)", "hybrid"},
		{"mobiles arbeiten in a benefit list", "<li>Gleitzeit, Langzeitkonto und mobiles Arbeiten</li>", "hybrid"},
		{"mobilem arbeiten", "die Möglichkeit zum mobilen Arbeiten/Homeoffice", "hybrid"},
		{"hybrides arbeiten", "Hybrides Arbeiten – Sie können Präsenz- und mobile Arbeitstage kombinieren", "hybrid"},
		{"hybrid zu arbeiten", "die Möglichkeit, hybrid zu arbeiten.", "hybrid"},
		{"arbeitsmodell hybrid", "Arbeitsmodell: hybrid mit regelmäßiger Office-Präsenz", "hybrid"},
		{"hybridmodell", "mit flexiblen Arbeitszeiten im Hybridmodell", "hybrid"},
		{"remote-anteil", "Remote-Anteil von 60 % nach der Einarbeitung", "hybrid"},
		{"telearbeit", "Die Möglichkeit zur Telearbeit ist gegeben.", "hybrid"},

		// Remote — home working as the WHOLE arrangement.
		{"100% homeoffice", "Kundenbetreuer Service Hotline (d/m/w) - deutschsprachig - 100% Homeoffice", "remote"},
		{"100% homeoffice möglich", "100% Homeoffice möglich; München, Deutschland", "remote"},
		{"vollständig remote", "Die Position ist vollständig remote.", "remote"},
		{"deutschlandweit remote", "Du arbeitest deutschlandweit remote.", "remote"},
		{"ausschließlich im homeoffice", "Sie arbeiten ausschließlich im Homeoffice.", "remote"},
		{"100% remote already english", "Die Stelle ist 100% remote.", "remote"},
		{"remote as one choice", "Teilzeit im Büro, hybrid oder 100% Homeoffice möglich", "remote"},

		// Onsite — home working explicitly ruled out.
		{"kein homeoffice", "Ihr zukünftiger Arbeitsort: 21465 Reinbek (vor Ort – kein Homeoffice)", "onsite"},
		{"homeoffice nicht möglich", "Homeoffice ist in dieser Position leider nicht möglich.", "onsite"},

		{"ausschließlich vor ort", "Wir arbeiten ausschließlich vor Ort in unserem Office in Frankfurt.", "onsite"},

		// Nothing — the words are present, the arrangement is not.
		{"onboarding month on site", "Im ersten Monat arbeitest du mit einem Pensum von 80 bis 100 % vor Ort.", ""},
		{"negated hybrid phrase", "Bitte beachten Sie: kein Homeoffice möglich.", "onsite"},
		{"mobile machinery is a trade", "Mechatroniker für mobile Arbeitsmaschinen (m/w/d)", ""},
		{"hybrid environments are technology", "Sicherheitsgrundsätze in hybriden Umgebungen sind dir vertraut", ""},
		{"hybrid projects are methodology", "Erfahrung in agilen oder hybriden Projekten", ""},
		{"company sells hybrid work", "Dienstleistungen für die hybride Arbeitswelt", ""},
		{"vor ort alone says nothing", "Unterstützung unserer Kunden vor Ort", ""},
		{"deutschlandweit alone says nothing", "Wir sind deutschlandweit an 40 Standorten vertreten.", ""},
		{"remote alone stays silent", "Fernwartung und Remote-Support für unsere Kunden", ""},

		// Review follow-ups (freehire#3574), synthetic.
		{"onsite split by markup", "Wir arbeiten ausschließlich vor<br>Ort.", "onsite"},
		{"postposed refusal of mobile work", "Mobiles Arbeiten ist in dieser Position nicht möglich.", ""},
		{"postposed refusal of a hybrid phrase", "Ein Homeoffice-Anteil ist leider nicht vorgesehen.", ""},
		{"negated onsite-only", "Wir arbeiten nicht ausschließlich vor Ort.", ""},
		{"no ban on home office", "Kein Homeoffice-Verbot, aber Präsenz im Team.", ""},
		{"not ruled out is an offer", "Mobiles Arbeiten ist nicht ausgeschlossen und wird nach Absprache angeboten.", "hybrid"},
		{"refusal in the next sentence", "Mobiles Arbeiten ist möglich. Homeoffice ist nicht möglich.", "hybrid"},
		{"von zu hause needs no other trigger", "Wir arbeiten vollständig von zu Hause; keine Einsätze beim Kunden.", "remote"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := WorkModeFromDescription(tc.desc); got != tc.want {
				t.Errorf("WorkModeFromDescription(%q) = %q, want %q", tc.desc, got, tc.want)
			}
		})
	}
}
