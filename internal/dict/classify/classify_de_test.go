package classify

import "testing"

// TestGermanCompoundHeads pins the German title vocabulary: a compound noun names
// its role in its LAST morpheme (Kranken|pfleger, Stapler|fahrer), so the head
// table matches a word's tail rather than the whole word. Every title here is a
// live posting from the German ATS crawl (coveto, rexx, dvinci, concludis, onlyfy,
// kenjo, xing), measured 2026-10-25.
func TestGermanCompoundHeads(t *testing.T) {
	cases := []struct {
		title, category, seniority string
	}{
		// Healthcare: the doctor, the nurse and their gender forms.
		{"Facharzt/Fachärztin (m/w/d) Innere Medizin", "healthcare", ""},
		{"Oberarzt (m/w/d) Neuropädiatrie", "healthcare", "senior"},
		{"Leitender Oberarzt (m/w/d) Kardiologie", "healthcare", "lead"},
		{"Chefarzt (m/w/d) Unfallchirurgie", "healthcare", "c_level"},
		{"Assistenzarzt Anästhesie m/w/d", "healthcare", "junior"},
		{"Pflegefachkraft (m/w/d)", "healthcare", ""},
		{"Gesundheits- und Krankenpfleger/in (m/w/d)", "healthcare", ""},
		{"Gesundheits- und Kinderkrankenpfleger/in (m/w/d)", "healthcare", ""},
		{"Pflegefachassistent_in", "healthcare", ""},
		{"Altenpflegehelfer:in", "healthcare", ""},
		{"Pflegedienstleitung (m/w/d) stationär", "healthcare", "lead"},
		{"Physiotherapeut (m/w/d)", "healthcare", ""},
		{"Psychologische/r Psychotherapeut/in (m/w/d)", "healthcare", ""},
		{"Hebamme (m/w/d)", "healthcare", ""},
		{"Medizinische Fachangestellte (m/w/d)", "healthcare", ""},
		{"Operationstechnische/r Assistent/in (m/w/d)", "healthcare", ""},
		{"Hörakustikerin / Hörgeräteakustiker (m/w/d)", "healthcare", ""},
		// Education and social services: the compound decides which.
		{"Erzieher | pädagogische Fachkraft", "education", ""},
		{"ErzieherInnen für unsere Kita", "education", ""},
		{"Kinderpfleger (m/w/d)", "education", ""},
		{"Heilerziehungspfleger (m/w/d)", "social_services", ""},
		{"Sozialpädagoge (m/w/d) Jugendhilfe", "social_services", ""},
		// The trades.
		{"Elektroniker (m/w/d)", "skilled_trades", ""},
		{"Ausbildung Mechatroniker", "skilled_trades", "intern"},
		{"Anlagenmechaniker SHK (m/w/d)", "skilled_trades", ""},
		{"Maler und Lackierer Geselle (m/w/d)", "skilled_trades", ""},
		{"Servicetechniker (m/w/d) im Außendienst", "skilled_trades", ""},
		{"Bauleiter (m/w/d) Tiefbau", "skilled_trades", "lead"},
		// Logistics, hospitality, cleaning, security.
		{"Staplerfahrer (m/w/d)", "logistics", ""},
		{"Fachkraft für Lagerlogistik (m/w/d)", "logistics", ""},
		{"Koch (m/w/d)", "hospitality", ""},
		{"Reinigungskraft (m/w/d)", "personal_services", ""},
		{"Raumpfleger (m/w/d)", "personal_services", ""},
		{"Kaufhausdetektiv (m/w/d) – Quereinsteiger willkommen", "public_safety", ""},
		{"Revierfahrer (m/w/d) – Quereinsteiger willkommen!", "public_safety", ""},
		// Office, money, law, sales.
		{"Sachbearbeiter/in im Bereich Wohngeld", "administration", ""},
		{"Kaufmann für Büromanagement (m/w/d)", "administration", ""},
		{"Finanzbuchhalter (m/w/d)", "finance", ""},
		{"Steuerfachangestellter (m/w/d)", "finance", ""},
		{"Rechtsanwalt (m/w/d) Arbeitsrecht", "legal", ""},
		{"Privatkundenberater (m/w/d)", "sales", ""},
		{"Vertriebspartner", "sales", ""},
		{"Personalberater (m/w/d)", "recruiting", ""},
		{"Personaldisponent (m/w/d)", "recruiting", ""},
		{"Projektleiter (m/w/d) Elektrotechnik", "project_management", "lead"},
		{"Projektingenieur (w/m/d) konstruktiver Ingenieurbau", "", ""},
		{"Bauingenieur (m/w/d)", "industrial_engineering", ""},
		{"Konstrukteur (m/w/d) Maschinenbau", "engineering_design", ""},
		{"Fachplaner Brandschutz", "engineering_design", ""},
		// Seniority words.
		{"Pflichtpraktikum - HR ERP-Systeme (m/w/d)", "", "intern"},
		{"Werkstudent (m/w/d) im Verkauf", "retail", "intern"},
		{"Auszubildende zur Kauffrau für Büromanagement", "administration", "intern"},
		{"Geschäftsführer (m/w/d)", "", "c_level"},
		{"Teamleiter (m/w/d) Kundenservice", "support", "lead"},
	}
	for _, tc := range cases {
		got := Parse(tc.title)
		if got.Category != tc.category || got.Seniority != tc.seniority {
			t.Errorf("Parse(%q) = {%q, %q}, want {%q, %q}", tc.title, got.Category, got.Seniority, tc.category, tc.seniority)
		}
	}
}

// TestGermanHeadsDoNotOverreach pins the words whose tail LOOKS like a head and is
// not one. Each was a candidate collision in the 2026-10-25 corpus.
func TestGermanHeadsDoNotOverreach(t *testing.T) {
	cases := []struct {
		title, category, seniority string
	}{
		// A companion, a semiconductor and a practice instructor end in "leiter".
		{"Alltagsbegleiter (m/w/d) in der Pflege", "healthcare", ""},
		{"Prozessingenieur Halbleiter (m/w/d)", "", ""},
		{"Praxisanleiter (m/w/d) Pflege", "healthcare", ""},
		// The executive's assistant states no executive grade.
		{"Assistenz der Geschäftsführung (m/w/d)", "administration", ""},
		// "With completed training" is a requirement, not an apprenticeship.
		{"Elektriker (m/w/d) mit abgeschlossener Ausbildung", "skilled_trades", ""},
		// A machine operator is not a leader; a landscape keeper is not a nurse.
		{"Maschinenführer (m/w/d)", "manufacturing", ""},
		{"Landschaftspfleger (m/w/d)", "", ""},
		{"Tierpfleger (m/w/d) im Zoo", "", ""},
		{"Baumpfleger (m/w/d)", "", ""},
		{"Pferdepfleger (m/w/d)", "", ""},
		// An IT technician is not a tradesman, and PLC programming stays unfiled.
		{"IT-Techniker (m/w/d)", "", ""},
		{"SPS-Programmierer (m/w/d)", "", ""},
		// A generic helper or employee names no craft.
		{"Helfer (m/w/d)", "", ""},
		{"Mitarbeiter (m/w/d)", "", ""},
		{"Fachkraft (m/w/d)", "", ""},
		{"Ausbildung (m/w/d)", "", "intern"},
		// Adjectives ending in a head's letters.
		{"Mitarbeiter mit normaler Arbeitszeit", "", ""},
	}
	for _, tc := range cases {
		got := Parse(tc.title)
		if got.Category != tc.category || got.Seniority != tc.seniority {
			t.Errorf("Parse(%q) = {%q, %q}, want {%q, %q}", tc.title, got.Category, got.Seniority, tc.category, tc.seniority)
		}
	}
}

// TestGermanHeadsNeverOverrideTheMainTable pins the placement: the head table is a
// fallback, so a title the main table already resolves keeps its answer.
func TestGermanHeadsNeverOverrideTheMainTable(t *testing.T) {
	for title, want := range map[string]string{
		"Sicherheitsmitarbeiter (m/w/d) Objektschutz": "public_safety",
		"Softwareentwickler (m/w/d)":                  "software_engineering",
		"Systemadministrator (m/w/d)":                 "devops",
		"Filialleiter (m/w/d)":                        "retail",
	} {
		if got := Parse(title).Category; got != want {
			t.Errorf("Parse(%q).Category = %q, want %q", title, got, want)
		}
	}
}

// TestGermanHoldoutCorrections pins what the hand-checked hold-out sample of
// 2026-10-25 caught the first draft getting wrong.
func TestGermanHoldoutCorrections(t *testing.T) {
	cases := []struct {
		title, category, seniority string
	}{
		// "ZFA" is also an authority's acronym; the clerk is the role.
		{"Sachbearbeiter/in Zentralstelle des Landes NRW für Flugabschiebungen (ZFA)", "administration", ""},
		// The assistant and the officer OF the management state no executive grade.
		{"Assistenz (m/w) der Geschäftsführung in Düsseldorf", "administration", ""},
		{"Referent (m/w/d) kaufmännische Geschäftsführung", "", ""},
		// A construction plant operator works a site, not a line.
		{"Baumaschinenführer (m/w/d)", "skilled_trades", ""},
		{"Schulbegleiter (m/w/d)", "education", ""},
		{"Medizinische/r Fachangestellte/r (m/w/d)", "healthcare", ""},
		{"Augenoptikermeister / Optikermeisterin (m/w/d)", "healthcare", ""},
		{"WERKSTUDENT* IM VERKAUF", "retail", "intern"},
		{"Pädagogische Fachkraft für Kindergartenbegleitung (m/w/d)", "education", ""},
		{"Assistenz Projektleitung Teilzeit/ Werkstudent/ Praktikum (m/w/d)", "project_management", "intern"},
		{"Praktikant / Werkstudent (m/w/d) Bauleitung - Schulbauprojekte", "skilled_trades", "intern"},
		{"Assistenz der Lagerleitung (m/w/d)", "", ""},
		{"Assistent:in der Abteilungsleitung im Kunden & Partnermanagement", "", ""},
		{"Revier- und Interventionsfahrer (m/w/d) in Stuttgart", "public_safety", ""},
		{"Ausbildung zum Chemielaboranten (m/w/d) zum 01.08.2027", "science", "intern"},
		{"Fachkraft für Arbeitssicherheit (m/w/d)", "occupational_safety", ""},
		{"Parkettleger (m/w/d) bis 21,70 € / Std.", "skilled_trades", ""},
		{"Langstielbaggerfahrer (m/w/d) | Verkehrswegebau", "skilled_trades", ""},
	}
	for _, tc := range cases {
		got := Parse(tc.title)
		if got.Category != tc.category || got.Seniority != tc.seniority {
			t.Errorf("Parse(%q) = {%q, %q}, want {%q, %q}", tc.title, got.Category, got.Seniority, tc.category, tc.seniority)
		}
	}
}
