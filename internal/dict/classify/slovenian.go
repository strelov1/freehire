package classify

import "regexp"

// Slovenian titles reach the BCS fallback (bcs.go) and are read by its machinery: the same
// folding (the "(m/ž)" notices cut, the diacritics dropped) and the same inflected stems.
// The languages are close, but the words a Slovenian title needs most are spelled
// otherwise: the senior is "višji", not "viši"; the junior "mlajši", not "mlađi"; the shift
// lead a "vodja izmene", not a "voditelj smjene"; the machine operator a "posluževalec" or
// an "upravljalec". So the Slovenian aliases are a table of their own, read after the BCS
// table resolved nothing — no BCS answer changes.
//
// The main table's Central and Eastern European block already reads most Slovenian role
// nouns (#3518): measured 2026-10-29 over 2,900 open MojeDelo postings and 3,621 ESS
// vacancies, 88% of the titles resolved a category, and the rest were mostly generic heads
// with no field ("Svetovalec", "Strokovni sodelavec", "Vodja oddelka", "Referent"), which
// stay unresolved as their English twins do. What this table adds is the machine floor by
// its heads and processes, the engineers by discipline, a few desks and trades, and the
// grades: 1.4% of MojeDelo's and 4.2% of ESS's titles stated one before.
//
// A Slovenian noun with a fleeting "e" ("posluževalec", genitive "posluževalca") is listed
// in both stems, because an ending is only ever appended to a stem (bcsEndings).

// slovenianCategoryTable lists the Slovenian category aliases, in precedence order, in the
// BCS alias syntax on the BCS-folded title. Each is filed where its English twin is.
var slovenianCategoryTable = []aliasEntry{
	// The engineers by discipline, before the fields: "Inženir za računalniški vid in
	// strojno učenje" is machine learning, not the machine floor. A bare inženir names
	// nothing, as in English.
	{"racunalnisk* vid*", "ml_ai"},
	{"strojn* ucenj*", "ml_ai"},
	{"omrezn* varnost*", "security"},
	{"informacijsk* varnost*", "security"},
	{"kibernetsk* varnost*", "security"},
	{"omrezn* tehnik*", "network_engineering"},
	{"inzenir* elektronik*", "hardware"},
	{"gradben* inzenir*", "industrial_engineering"},
	{"gradbiscn* inzenir*", "industrial_engineering"},
	{"inzenir* gradbenistv*", "industrial_engineering"},
	{"inzenir* s podrocj* gradbenistv*", "industrial_engineering"},
	{"inzenir* strojnistv*", "industrial_engineering"},
	{"inzenir* za kakovost*", "industrial_engineering"},
	{"inzenir* kakovost*", "industrial_engineering"},
	{"statik*", "industrial_engineering"},
	// The clinical laboratory, as SOC files it: healthcare.
	{"laboratorijsk* biomedicin*", "healthcare"},
	{"svetovalec za sap", "solutions_engineering"},
	{"svetovalka za sap", "solutions_engineering"},
	// The role nouns.
	{"posluzevalec", "manufacturing"},
	{"posluzevalc*", "manufacturing"},
	{"posluzevalk*", "manufacturing"},
	{"upravljalec stroj*", "manufacturing"},
	{"upravljalc* stroj*", "manufacturing"},
	{"upravljavec stroj*", "manufacturing"},
	{"upravljavc* stroj*", "manufacturing"},
	{"upravljalec naprav*", "manufacturing"},
	{"upravljalec linij*", "manufacturing"},
	{"upravljalc* linij*", "manufacturing"},
	{"zagarist*", "manufacturing"},
	{"logisticn* delavec", "logistics"},
	{"logisticn* delavc*", "logistics"},
	{"logisticn* delavk*", "logistics"},
	{"avtoelektrikar*", "skilled_trades"},
	{"asfalter*", "skilled_trades"},
	{"asfaltersk*", "skilled_trades"},
	// The store manager: the Slovenian "poslovodja" heads a shop, as "Store Manager" does.
	// Matched as written: the BCS "poslovođa" is also a site foreman, and #3778 left it
	// unresolved.
	{"=poslovodj*", "retail"},
	{"saldakontist*", "finance"},
	{"saldakont*", "finance"},
	{"varuhinj*", "healthcare"},
	{"institucionaln* varstv*", "healthcare"},
	// The centre for social work. Bare "CSD" is also a hospital's sterile department.
	{"na csd", "social_services"},
	{"v csd", "social_services"},
	{"center* za socialn* del*", "social_services"},
	// The fields: the machine floor's processes and lines, the warehouse, the pharmacy,
	// the kitchen and the laundry, the call centre.
	{"proizvodn* linij*", "manufacturing"},
	{"robotiziran* linij*", "manufacturing"},
	{"robotsk* linij*", "manufacturing"},
	{"pakirn* linij*", "manufacturing"},
	{"galvansk* linij*", "manufacturing"},
	{"brizganj*", "manufacturing"},
	{"brizgaln*", "manufacturing"},
	{"upogibn*", "manufacturing"},
	{"kriviln*", "manufacturing"},
	{"lasersk* razrez*", "manufacturing"},
	{"mehansk* obdelav*", "manufacturing"},
	{"toplotn* obdelav*", "manufacturing"},
	{"povrsinsk* obdelav*", "manufacturing"},
	{"obdelav* les*", "manufacturing"},
	{"litj*", "manufacturing"},
	{"livarn*", "manufacturing"},
	{"taljenj*", "manufacturing"},
	{"skladisc*", "logistics"},
	{"posiljk*", "logistics"},
	{"lekarn*", "healthcare"},
	{"strezb*", "hospitality"},
	{"slascicar*", "hospitality"},
	{"slascicarsk*", "hospitality"},
	{"pralnic*", "hospitality"},
	{"klicn* centr*", "support"},
}

// slovenianSeniorityTable lists the Slovenian grades. Višji is senior only in the forms that
// grade a person: "višje" and "višjo" are also "higher" ("z višjo izobrazbo"). Vodja alone
// is any manager and states no grade, as "manager" does; only the shift, team and group
// leads are lead. A plain "direktor" states none either.
var slovenianSeniorityTable = []aliasEntry{
	{"financn* direktor*", "c_level"},
	{"studentsk* del*", "intern"},
	{"studentsk* priloznost*", "intern"},
	{"pripravnistv*", "intern"},
	{"vodja izmen*", "lead"},
	{"vodja skupin*", "lead"},
	{"vodja ekip*", "lead"},
	{"izmenovodj*", "lead"},
	{"skupinovodj*", "lead"},
	{"vodiln* razvojn*", "lead"},
	{"vodiln* inzenir*", "lead"},
	{"vodiln* razvijalec", "lead"},
	{"vodiln* programer*", "lead"},
	{"visji", "senior"},
	{"visja", "senior"},
	{"visjega", "senior"},
	{"visjemu", "senior"},
	{"mlajsi", "junior"},
	{"mlajsa", "junior"},
	{"mlajsega", "junior"},
	{"zacetnik*", "junior"},
	{"zacetnic*", "junior"},
}

// slovenianAssistantOfLead is the assistant or deputy OF a lead or an executive — "Pomočnik
// vodje delavnice", "Namestnik vodje poslovne enote", "Pomočnik generalnega direktorja" —
// which states no grade of its own. It is cut, on the folded title, before the grade match.
var slovenianAssistantOfLead = regexp.MustCompile(`\b(?:pomo[cč]ni(?:k|ca)|namestni(?:k|ca)|asistent(?:ka)?)\s+(?:(?:glavnega|generalnega|izvr[sš]nega|finan[cč]nega)\s+)?(?:direktor|vodj|vodij)\p{L}*`)

var (
	slovenianCategory  = compileBCS(slovenianCategoryTable)
	slovenianSeniority = compileBCS(slovenianSeniorityTable)
)
