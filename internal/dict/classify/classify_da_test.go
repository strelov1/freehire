package classify

import "testing"

// TestDanishCompoundHeads pins the Danish title vocabulary. Danish, like German and
// Dutch, names a role in the LAST morpheme of a compound (lager|medarbejder,
// social|pædagog, software|udvikler), so the head table matches a word's tail. Every
// title here is a live posting from the Danish boards (jobindex, jobnet, jobdanmark),
// measured 2026-10-26.
func TestDanishCompoundHeads(t *testing.T) {
	cases := []struct {
		title, category, seniority string
	}{
		// IT.
		{"Softwareudvikler til maritime kommunikationsløsninger", "software_engineering", ""},
		{"IT-udvikler til automatisering, RPA og AI i Lolland Kommune", "software_engineering", ""},
		{"Softwareingeniør", "software_engineering", ""},
		{"IT-supporter", "it_support", ""},
		{"IT-arkitekt til sikring af fremtidens Sejlads", "architecture", ""},
		{"Dataanalytiker til bekæmpelse af skatteøkonomisk kriminalitet", "data_analytics", ""},
		{"Underviser til uddannelser i IT-sikkerhed – vær med til at forme fremtidens digitale forsvar", "security", ""},
		{"IT-projektleder til Energi Fyns digitale udvikling", "project_management", "lead"},
		// The machine shop programs a machining centre, not software.
		{"CNC-Programmør til aftenhold -Esbjerg", "manufacturing", ""},
		{"CNC-fræseoperatør / programmør – 5-akset fræsning og spændende specialopgaver", "manufacturing", ""},
		{"Laseroperatør/-programmør søges i Herning!", "manufacturing", ""},
		{"Automatik- og PLC-programmør til KVM A/S", "industrial_engineering", ""},

		// Engineering.
		{"Konstruktionsingeniør til Banedanmarks broprojekter", "industrial_engineering", ""},
		{"El-ingeniør til Danmarks største raffinaderi", "industrial_engineering", ""},
		{"Arbejdsmiljørådgiver ved Ryes kaserne i Fredericia", "occupational_safety", ""},

		// Care: the nurse, the SOSU grades, the therapists.
		{"Sygeplejerske", "healthcare", ""},
		{"Sygeplejersker til Perioperativ Enhed, Thisted", "healthcare", ""},
		{"Social- og sundhedshjælper i dagvagt til Team Midtthy", "healthcare", ""},
		{"SOSU-hjælper til fast aftenvagt – Vil du være med på Borgerteams holdet?", "healthcare", ""},
		{"SSA og SSH til Plejecenter Marienlyst pr. 1. januar 2027", "healthcare", ""},
		{"SSA i dagvagt søges til Filskov Friplejehjem", "healthcare", ""},
		{"Fast SSH Dagvagt", "healthcare", ""},
		{"Fysioterapeut til klinik søges", "healthcare", ""},
		{"Reservelæge i klinisk farmakologi i Region Sjælland", "healthcare", ""},
		{"Tandplejer søges til Tandlægehuset Køge", "healthcare", ""},
		{"Ledsager søges til kvinde i Solrød Strand", "social_services", ""},
		{"Socialpædagog søges til Psykiatrisk Døgnafsnit 06/07", "social_services", ""},
		{"Socialrådgiver til Familierådgivningen", "social_services", ""},

		// Education.
		{"Pædagog", "education", ""},
		{"Pædagogmedhjælpere til børnehaven på Børnegården Uhrhøj", "education", ""},
		{"Klasselærer til 2. klasse på Kristofferskolen", "education", ""},
		{"Dagplejer til Rødding", "education", ""},

		// The trades, the factory floor, the warehouse.
		{"Tømrersvend søges til HeFaByg", "skilled_trades", ""},
		{"VVS-montør til varmeinstallation og service i Varde", "skilled_trades", ""},
		{"Klejnsmed søges", "skilled_trades", ""},
		{"Lastvognsmekaniker til Stiholt Erhverv Kolding", "skilled_trades", ""},
		{"Anlægsgartner til belægning og grøn drift i Roskilde", "skilled_trades", ""},
		{"Servicetekniker", "skilled_trades", ""},
		{"Produktionsmedarbejder", "manufacturing", ""},
		{"Industritekniker søges til Acoplastic A/S", "manufacturing", ""},
		{"Procesoperatør til fast nathold søges", "manufacturing", ""},
		{"Lagermedarbejder", "logistics", ""},
		{"LASTBILCHAUFFØR SØGES - Bliv en del af Recycon Element", "logistics", ""},
		{"Der søges 20 identiske lager- og logistikoperatører til Forsvarets Forsyningscentre i DK.", "logistics", ""},

		// Hospitality, retail, personal services.
		{"Tjener søges", "hospitality", ""},
		{"Overtjener søges til Sauntehus Slotshotel", "hospitality", ""},
		{"Køkkenmedhjælper til Flyvestation Aalborg", "hospitality", ""},
		{"Salgsassistentelev til Frugt og Grønt - Korsør - Føtex | Salling Group", "retail", "intern"},
		{"Butiksassistent under 18 år - Kastrup", "retail", ""},
		{"Frisør (fuld-/deltid)", "personal_services", ""},
		{"Rengøringsassistent til kontor i Greve", "personal_services", ""},

		// The office, finance, sales.
		{"Regnskabschef", "finance", ""},
		{"Ambitiøs og erfaren revisor til PwC Aalborg", "finance", ""},
		{"Lægesekretær/Sundhedsadministrativ koordinator, Akut- og Traumecenter", "administration", ""},
		{"Kontorelev i Offentlig Administration – start 1. marts 2027", "administration", "intern"},
		{"Kundeservicemedarbejder", "support", ""},
		{"Salgsleder", "sales", "lead"},
		{"Driftsleder", "operations", "lead"},

		// Grades only.
		{"Studentermedhjælper til Marketing & Kommunikation", "marketing", "intern"},
		{"Bliv leder for et stærkt specialistteam i Customer Care hos Circle K", "support", "lead"},
	}
	for _, c := range cases {
		got := Parse(c.title)
		if got.Category != c.category || got.Seniority != c.seniority {
			t.Errorf("Parse(%q) = {%q, %q}, want {%q, %q}", c.title, got.Category, got.Seniority, c.category, c.seniority)
		}
	}
}

// TestDanishHeadsDoNotOverreach pins the words whose tail or abbreviation LOOKS like a
// Danish head and is not that role — in Danish and in the other languages that reach
// the same fallback.
func TestDanishHeadsDoNotOverreach(t *testing.T) {
	cases := []struct {
		title, category, seniority string
	}{
		{"Kirketjener i Randers", "", ""},                                                     // the verger, not a waiter
		{"Miljøsagsbehandler til Landbrugsområdet", "", ""},                                   // a caseworker of any field
		{"Faglig stærk social- og sundhedsassistent som oplæringsvejleder", "healthcare", ""}, // "vejleder" is no lead
		{"Order to Cash Leader SSA (Sub Sahara Africa)", "", ""},                              // SSA the region
		{"Handläggande VVS-ingenjör WSP Stockholm", "", ""},                                   // the Swedish HVAC designer
		{"Mitarbeiter mit normaler Arbeitszeit", "", ""},                                      // German "normaler", not a painter
		{"Programador CNC", "", ""},                                                           // the Spanish form stays unresolved
	}
	for _, c := range cases {
		got := Parse(c.title)
		if got.Category != c.category || got.Seniority != c.seniority {
			t.Errorf("Parse(%q) = {%q, %q}, want {%q, %q}", c.title, got.Category, got.Seniority, c.category, c.seniority)
		}
	}
}
