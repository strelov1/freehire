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
	"material handler", "package handler", "cdl driver",
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
	"maintenance worker", "parking attendant", "flight attendant",
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
