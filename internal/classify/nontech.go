package classify

import (
	"strings"

	"github.com/strelov1/freehire/internal/wordmatch"
)

// nonTechTitleTerms is a curated set of confidently non-technical role nouns
// found in job titles, beyond the four non-tech categories the categoryTable
// already resolves (marketing/sales/support/management). It exists to place the
// large tail of non-engineering roles that generic ATS boards pour into the
// catalogue — healthcare, trades, hospitality, retail, logistics, education,
// personal care, facilities — which the tech-focused categoryTable leaves empty.
//
// The same doctrine as the rest of classify: whole-word match, never guess. Two
// rules keep it from ever shadowing a technical role:
//   - No term that also occurs in tech titles. Bare "engineer"/"technician"/
//     "analyst"/"driver" (device driver)/"server" (backend)/"warehouse" (data
//     warehouse) are deliberately absent; ambiguous roles use a full non-tech
//     phrase ("truck driver", "line cook") instead.
//   - This detector is consulted only after the tech category dictionary is
//     silent (see jobderive), so tech evidence always wins.
var nonTechTitleTerms = []string{
	// Healthcare & care. "…technician" collides with IT/field-service technician, so
	// only anchored forms ("pharmacy technician", "surgical technician") are listed.
	// The behavior-technician cluster is the largest single unclassified group in the
	// catalogue (~26k), all ABA therapy; "rbt" is its universal abbreviation and is not
	// a word in any technical title.
	"behavior technician", "registered behavior technician", "rbt",
	"behavioral health", "speech language pathologist", "language pathologist",
	"therapy assistant", "care aide", "care assistant", "social worker",
	"nurse", "nursing", "registered nurse", "nurse practitioner",
	"certified nursing assistant", "cna", "lpn", "licensed practical nurse",
	"caregiver", "caretaker", "home health aide", "home health", "hospice",
	"veterinary", "veterinarian", "dentist", "dental hygienist", "dental assistant",
	"pharmacist", "pharmacy technician", "phlebotomist", "phlebotomy", "paramedic",
	"medical assistant", "certified medical assistant", "medical scribe",
	"medical technologist", "patient care", "patient access",
	"surgical technologist", "surgical technician", "scrub tech",
	"physical therapist", "occupational therapist", "respiratory therapist",
	"speech therapist", "massage therapist", "radiologic technologist", "sonographer",
	// Skilled trades. "…engineer"/"…technician" excluded; "hvac technician" anchored.
	"electrician", "plumber", "welder", "carpenter", "machinist", "millwright",
	"forklift", "ironworker", "iron worker", "laborer", "general labor",
	"pipefitter", "stone mason", "brick mason", "bricklayer", "roofer", "hvac",
	"hvac technician", "mechanic", "boilermaker", "sheet metal",
	// Hospitality & food service. Bare "chef" is deliberately absent — it collides
	// with Progress Chef (config-management), which would mislabel a DevOps/SRE
	// title the tech dictionary did not place; the cook terms cover food service.
	"cook", "line cook", "prep cook", "fry cook", "grill cook", "food service",
	"barista", "bartender", "dishwasher", "housekeeper", "housekeeping", "banquet",
	"waiter", "waitress", "busser", "concierge", "valet",
	// Retail & warehouse. Bare "warehouse" excluded (data-warehouse engineer); only
	// anchored role forms.
	"cashier", "stocker", "merchandiser", "retail associate", "retail sales",
	"sales associate", "sales clerk", "store associate", "store clerk",
	"warehouse associate", "warehouse worker", "order picker", "picker", "packer",
	"material handler", "package handler", "cdl driver", "machine operator",
	"crew member", "car rental",
	// Personal care & fitness
	"pilates instructor", "yoga instructor", "fitness instructor",
	"personal trainer", "cosmetologist", "hair stylist", "hairstylist",
	"barber", "esthetician", "manicurist", "nail technician",
	// Education & childcare
	"teacher", "substitute teacher", "teaching assistant", "tutor", "preschool",
	"childcare", "child care", "daycare", "camp counselor", "paraprofessional",
	// Office & finance administration. Anchored forms only ("data entry clerk", not
	// bare "data"; "loan officer", not bare "officer").
	"paralegal", "bookkeeper", "accountant", "accounting clerk", "payroll clerk",
	"payroll specialist", "accounts payable", "accounts receivable", "teller",
	"bank teller", "loan officer", "underwriter", "claims adjuster",
	"administrative assistant", "office assistant", "data entry clerk", "file clerk",
	// Facilities & cleaning
	"janitor", "janitorial", "cleaner", "custodian", "custodial", "groundskeeper",
	"maintenance worker", "maintenance technician", "parking attendant", "flight attendant",
	// Security & transport
	"security guard", "truck driver", "delivery driver", "bus driver", "courier",
	// Front-of-house administration
	"receptionist", "front desk",
	// Russian non-technical roles (the trudvsem/hh tail arrives in Russian, so the
	// English terms above never fire and these jobs fall to unknown instead of
	// non-tech). Same doctrine: unambiguous role nouns only, whole-word (Cyrillic
	// boundaries handled by wordmatch.UnicodeBoundary). Deliberately ABSENT — the
	// Russian ambiguous words that mirror the English exclusions: bare "инженер"
	// (engineer), "техник" (IT/field technician), "оператор" (operator), bare
	// "администратор"/"директор"/"менеджер"/"аналитик"/"специалист"/"мастер".
	// "бухгалтер" is absent too — it already resolves to the finance category.
	// Healthcare & care
	"медсестра", "медицинская сестра", "медбрат", "фельдшер", "санитар", "санитарка",
	"сиделка", "няня",
	// Skilled trades
	"сварщик", "электрогазосварщик", "слесарь", "сантехник", "электромонтер",
	"электромонтёр", "электрик", "токарь", "фрезеровщик", "маляр", "штукатур",
	"каменщик", "бетонщик", "арматурщик", "стропальщик", "монтажник", "машинист",
	"тракторист",
	// Food service
	"повар", "пекарь", "кондитер", "официант", "официантка", "бармен",
	"посудомойщик", "кухонный рабочий",
	// Retail & warehouse
	"продавец", "кассир", "продавец-кассир", "продавец-консультант", "кладовщик",
	"комплектовщик", "грузчик", "упаковщик", "фасовщик", "приёмщик", "товаровед",
	"мерчендайзер", "администратор магазина", "директор магазина",
	// Cleaning, facilities & security
	"уборщик", "уборщица", "дворник", "разнорабочий", "подсобный рабочий", "вахтер",
	"вахтёр", "сторож", "охранник", "консьерж",
	// Education & personal care
	"воспитатель", "учитель", "логопед", "парикмахер", "маникюрша", "косметолог",
	// Transport
	"водитель", "курьер", "экспедитор",
	// Portuguese (BR) non-technical roles (the gupy/BR tail). Same doctrine; bare
	// ambiguous words absent: "técnico" (technician), "operador" (operator),
	// "analista"/"assistente"/"vendedor"/"professor"/"segurança" (infosec collision).
	// Retail & warehouse
	"operador de caixa", "operador de loja", "repositor", "estoquista", "frentista",
	"atendente de loja",
	// Cleaning & facilities
	"auxiliar de limpeza", "faxineiro", "zelador", "porteiro",
	// Food service
	"cozinheiro", "auxiliar de cozinha", "padeiro", "açougueiro", "garçom", "copeiro",
	// Trades
	"pedreiro", "pintor", "soldador", "eletricista", "encanador", "mecânico",
	"servente", "ajudante geral", "jardineiro",
	// Care, transport & front-of-house
	"cuidador", "enfermeiro", "técnico de enfermagem", "auxiliar de enfermagem",
	"babá", "motorista", "motoboy", "entregador", "vigilante", "camareira",
	"recepcionista",

	// ---- Second mining wave (cmd/mine-titles over the residual mass, 2026-07-28) ----
	//
	// The report ranks clusters by volume, and volume alone would have wrecked the
	// catalogue: its largest groups were "team member", "systems engineer" (80
	// sources), "team lead" (93), "tech lead" (78), "technical lead" (65),
	// "software engineering" (55), "product management", "data center", and the bare
	// seniorities "vice president"/"senior director"/"senior associate". Those name
	// the roles this board exists to carry and are deliberately absent. So are the
	// borderline "quality/process/project/service engineer" and "service technician",
	// which are mostly industrial but collide with QA and IT field-service titles;
	// they need their own decision, not a bulk import.
	//
	// Absent for a different reason: "banco de talentos" (29 sources), "general
	// application" (34), "ausbildung zum" (18), "jovem aprendiz", "sökes till". These
	// are not non-technical roles but ABSENT ones — speculative-application
	// placeholders — and a technical company's talent pool matches them exactly.
	// "são paulo" (23 sources) is a city, and "and older" is the tail of an age
	// requirement; neither is a role at all.
	//
	// Non-software engineering. Anchored to a physical discipline, so a software role
	// in the same domain ("Manufacturing Execution Systems Engineer") does not contain
	// the phrase. "maintenance engineer" is deliberately NOT here: "Software
	// Maintenance Engineer" already stands as a negative in this file's tests.
	"electrical engineer", "mechanical engineer", "civil engineer",
	"structural engineer", "manufacturing engineer", "quantity surveyor",
	// Healthcare, continued. "mental health", "clinical research", "primary care" and
	// "advanced practice" are domain words health-tech companies put in their own
	// engineering titles, so only the anchored role forms are listed.
	"mental health technician", "mental health counselor", "mental health therapist",
	"mental health associate", "clinical research coordinator",
	"clinical research associate", "primary care physician", "primary care provider",
	"advanced practice provider", "advanced practice nurse",
	"physician assistant", "family medicine", "medical director", "care coordinator",
	"dietary aide", "radiology technologist", "mri technologist",
	// Hospitality & food service, continued. Bare "chef" stays absent (Progress Chef,
	// the config-management tool); the anchored brigade titles do not collide.
	"sous chef", "chef de partie", "restaurant team", "room attendant", "guest service",
	// Automotive service
	"automotive technician", "automotive service", "service advisor", "auto body",
	"oil change", "lube technician",
	// Production & heavy equipment. Bare "production" names an SRE-adjacent role
	// ("Production Engineer"), so only the anchored non-engineering forms are listed.
	"production operator", "production associate", "production supervisor",
	"maintenance supervisor", "equipment operator", "heavy equipment",
	// Banking, insurance & the retail floor. "wealth management" and "client advisor"
	// are absent: both are ordinary fintech engineering-title words.
	"personal banker", "relationship banker", "associate banker", "financial advisor",
	"insurance agent", "merchandise associate", "retail keyholder", "beauty advisor",
	"store mgr", "store manager",
	// Education, laboratory & facilities
	"adjunct faculty", "assistant professor", "lab technician",
	"environmental services", "community liaison",
	// The Spanish/Portuguese and Russian tails of the same wave
	"auxiliar administrativo", "охране труда", "охрана труда",
}

// ConfirmedNonTech reports whether a title states a non-technical role AND nothing
// technical overrides it. It exists because IsNonTech alone is not a safe basis for
// removing a posting: the dictionary matches its terms anywhere in a title, and it was
// written on the contract stated above — consulted only after the tech check. Callers
// that DELETE on this signal (the ingest filter, the prune rule) must go through here,
// so the precedence cannot be forgotten in one of them and not the other.
//
// `engineering_design` vetoes deletion on its own, without technical evidence. That
// category and this dictionary describe the same physical trades from two sides — the
// list anchors "hvac", "sheet metal", "machinist"; the category resolves the draughting
// titles those same employers post — so a word match here is not the accidental kind
// the veto was built for. What is decisive is that a resolved category is a DELIBERATE
// placement: the title named a discipline, the catalogue keeps the posting under the
// `engineering_design` facet, and only `is_tech=false` follows from it. While these
// titles lived in `design`, TechCategories supplied this veto for free; splitting them
// out would otherwise turn an "HVAC Designer" away at ingest and hard-delete the
// stored rows through prune.
func ConfirmedNonTech(title string, hasTechEvidence bool) bool {
	if hasTechEvidence || Parse(title).Category == "engineering_design" {
		return false
	}
	return IsNonTech(title)
}

// NonTechTerms returns the curated non-tech title terms. Exposed so the catalogue
// miner can check its stop-word list against them: a stop word that is also a token
// of a known non-tech role would drop every word pair built from that phrase, hiding
// the whole role family from mining rather than merely filtering it. Returns a copy;
// the dictionary stays immutable.
func NonTechTerms() []string {
	return append([]string(nil), nonTechTitleTerms...)
}

// ptGenderSuffix strips the Brazilian-Portuguese inclusive gender parentheticals
// ("operador(a)", "enfermeiro(a)") so a phrase term matches the common written form.
// Without it "operador(a) de caixa" would not match "operador de caixa".
var ptGenderSuffix = strings.NewReplacer("(a)", "", "(o)", "", "(as)", "", "(os)", "", "(a/o)", "")

// IsNonTech reports whether a job title states a confidently non-technical role,
// matching any nonTechTitleTerms term on word boundaries. It never guesses: a
// title it cannot confidently place returns false. It resolves ONLY non-technical
// roles — a technical title yields false — so it can feed the is_tech derivation
// without risking a technical job being mislabelled.
func IsNonTech(title string) bool {
	lower := ptGenderSuffix.Replace(strings.ToLower(title))
	for _, term := range nonTechTitleTerms {
		if wordmatch.Contains(lower, term, wordmatch.UnicodeBoundary) {
			return true
		}
	}
	return false
}
