package classify

import "testing"

// TestDutchCompoundHeads pins the Dutch title vocabulary. Dutch, like German, names a
// role in the LAST morpheme of a compound (servicedesk|medewerker, applicatie|beheerder,
// wijk|verpleegkundige), so the head table matches a word's tail. Every title here is a
// live posting from the Dutch crawl (eures, recruitee, whatjobs-nl), measured 2026-10-25.
func TestDutchCompoundHeads(t *testing.T) {
	cases := []struct {
		title, category, seniority string
	}{
		// IT: the functional and technical administrators, the service desk.
		{"Functioneel Beheerder Sociaal Domein", "business_analysis", ""},
		{"Functioneel Applicatiebeheerder ERP", "business_analysis", ""},
		{"Applicatiebeheerder Publieke Zaken", "it_support", ""},
		{"ICT Servicedeskmedewerker", "it_support", ""},
		{"Medewerker Servicedesk (IT & Facilitair", "it_support", ""},
		{"Werkplekbeheerder", "it_support", ""},
		{"Netwerkbeheerder (Infrastructuur)", "network_engineering", ""},
		{"Systeem- & Netwerkbeheerder", "network_engineering", ""},
		{"systeembeheerder junior/medior", "devops", "junior"},
		{"Business Analist Data", "business_analysis", ""},
		{"Senior Business & Informatieanalist", "business_analysis", "senior"},
		{"Data Analist", "data_analytics", ""},
		// The main table answers first: a marketing data analyst files under marketing.
		{"Marketing Data Analist", "marketing", ""},
		{"Testautomatiseerder eIDAS", "qa", ""},
		{"Senior Domeinarchitect Locatie", "architecture", "senior"},
		{"Informatiebeveiligingsspecialist", "security", ""},
		{"Adviseur Informatiebeveiliging", "security", ""},
		{"Informatiebeveiliger", "security", ""},
		{"Applicatieontwikkelaar", "software_engineering", ""},
		{"(.NET) C# Ontwikkelaar", "software_engineering", ""},
		{"Senior WSO2 Ontwikkelaar/Applicatiebeheerder", "software_engineering", "senior"},
		{"Medior/Senior Softwareontwikkelaar (TypeScript) (m/v/x)", "software_engineering", "senior"},
		{"SEA Marketeer", "marketing", ""},
		{"Online (Medior) Marketeer", "marketing", "middle"},
		// A developer of policy, property or an area writes no software.
		{"Beleidsontwikkelaar Wonen", "", ""},
		{"Projectontwikkelaar Vastgoed", "", ""},
		// The CNC programmer works a machining shop; a venue's programmer curates.
		{"CAD/CAM Programmeur", "manufacturing", ""},
		{"CNC Programmeur / Werkvoorbereider (Tekenaar", "manufacturing", ""},
		{"Live Programmeur", "", ""},

		// Care: the nurse, the carer, the support worker, the psychologist.
		{"Verpleegkundige | Somatiek", "healthcare", ""},
		{"Nachtverpleegkundige/Begeleider Psychiatrie & Verslaving", "healthcare", ""},
		{"Verzorgende IG | Somatiek", "healthcare", ""},
		{"Helpende (Plus) | Dementie", "healthcare", ""},
		{"Tandartsassistent Amsterdam - 21/36 uur per week", "healthcare", ""},
		{"Basisarts Spoedeisende Psychiatrie Amsterdam-Amstelland", "healthcare", ""},
		{"GZ-Psycholoog", "healthcare", ""},
		{"Orthopedagoog Generalist Polikliniek Emerhese", "social_services", ""},
		{"Parttime begeleider", "social_services", ""},
		{"Persoonlijk Begeleider Intensief", "social_services", ""},
		{"Woonbegeleider", "social_services", ""},
		{"Pedagogisch Medewerker - Amersfoort", "education", ""},
		{"Docent NT2 Inburgering", "education", ""},
		// A process or project companion is not a care worker, and no lead.
		{"Procesbegeleider Data", "", ""},

		// The trades, construction and logistics.
		{"Uitvoerder GWW KWS", "skilled_trades", ""},
		{"Timmerman | Timmervrouw", "skilled_trades", ""},
		{"(Leerling) Allround timmerman - Kamerik", "skilled_trades", "intern"},
		{"Werkvoorbereider Productie", "engineering_design", ""},
		{"Calculator Elektrotechniek", "industrial_engineering", ""},
		{"Vrachtwagenchauffeur Winkeldistributie", "logistics", ""},
		{"Chauffeur C", "logistics", ""},
		{"Magazijnmedewerker avond | Jij zorgt voor een blije klant!", "logistics", ""},
		{"Logistiek Medewerker (Parttime)", "logistics", ""},
		{"Bezorger Houten | Meubels", "logistics", ""},
		{"Productiemedewerker Bandenvernieuwing", "manufacturing", ""},

		// Hospitality, retail, cleaning, security.
		{"Zelfstandig werkend kok, Lunchkok", "hospitality", ""},
		{"Leerling Kok", "hospitality", "intern"},
		{"Horecamedewerker (Bijbaan)", "hospitality", ""},
		{"Verkoopmedewerker Amsterdam Zuidas", "retail", ""},
		{"Medewerker Schoonmaak", "personal_services", ""},
		{"Security Officer/ Beveiliger", "public_safety", ""},
		{"Middelbaar Veiligheidskundige (MVK)", "occupational_safety", ""},
		{"KAM-Coördinator", "occupational_safety", ""},

		// The office, money and law.
		{"Administratief Medewerker ICT", "administration", ""},
		{"Secretaresse", "administration", ""},
		{"Allround Financieel Medewerker", "finance", ""},
		{"Junior Belastingadviseur", "finance", "junior"},
		{"HR Adviseur (24 - 32 uur)", "hr", ""},
		{"Advocaat-medewerker M&A", "legal", ""},
		{"Relatiebeheerder MKB", "sales", ""},
		{"Accountmanager Detachering", "sales", ""},
		{"Klantenservice", "support", ""},
		{"Parttime Klantenservice Medewerker (Studenten)", "support", ""},
		{"Medewerker ICT", "it_support", ""},
		{"Projectleider Informatisering", "project_management", "lead"},
		{"Projectleider Software Engineering", "software_engineering", "lead"},

		// Grades.
		{"Medior UX Designer", "design", "middle"},
		{"Stagiair Webdeveloper", "", "intern"},
		{"Afstudeerstage Data & AI", "", "intern"},
		{"(Afstudeer)Stage Communicatie", "", "intern"},
		{"HBO Meewerkstage Social Media Marketing", "marketing", "intern"},
		{"Traineeship Functioneel Beheer Domein Samenleving", "", "intern"},
		{"BBL Leerling IT", "", "intern"},
		{"Teamleider Servicedesk", "it_support", "lead"},
		{"Hoofd Engineering", "", "lead"},
		{"Adjunct Teamhoofd Externe Inhuur", "", "lead"},
		{"Meewerkend Voorman", "", "lead"},
		{"Leidinggevende Keuken", "", "lead"},
		{"Algemeen Directeur", "", "c_level"},
		{"Directeur-bestuurder Woningcorporatie", "", "c_level"},
		// A plain "directeur" reads as the English "director" does: no grade.
		{"Businessteam Directeur Wageningen", "", ""},
		// "-leider" is a lead only where it leads: a companion and a trainer are not,
		// nor is the assistant of a lead.
		{"Praktijkopleider Verpleegkunde", "", ""},
		{"Assistent Bedrijfsleider", "", ""},
	}
	for _, c := range cases {
		got := Parse(c.title)
		if got.Category != c.category || got.Seniority != c.seniority {
			t.Errorf("Parse(%q) = {cat %q, sen %q}, want {cat %q, sen %q}",
				c.title, got.Category, got.Seniority, c.category, c.seniority)
		}
	}
}

// TestDutchHeadsLeaveOtherLanguagesAlone pins the collisions the Dutch tails would
// otherwise make in English and German titles, which reach the same fallback when the
// main and German tables are silent.
func TestDutchHeadsLeaveOtherLanguagesAlone(t *testing.T) {
	for _, title := range []string{
		"Stage Manager",           // the theatre's, not an intern
		"Early-stage Investor",    // a startup stage
		"Bangkok Regional Office", // ends in "-kok"
		"Parts Advisor",
		"Director, Late Stage Clinical Development",
		"Launch Engineer, Stage 0 Propellant Generation",
		"Financial Solutions Advisor Stage II - Registration Candidate",
		"IBM InfoSphere Data Stage Consultant",
		"First Stage Researcher (R1)",
		"Kältetechniker m/w/d bis zu 35 Urlaubstage", // German "Tage", days
		"Mitarbeiter Produktion (m/w/d) im Bereich Adjustage",
	} {
		if got := Parse(title); got.Seniority == "intern" || got.Category == "hospitality" || got.Category == "healthcare" {
			t.Errorf("Parse(%q) = %+v, want the Dutch heads silent", title, got)
		}
	}
}
