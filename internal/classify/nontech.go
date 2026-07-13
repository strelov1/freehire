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
	// Healthcare & care
	"nurse", "nursing", "caregiver", "caretaker", "home health aide",
	"veterinary", "veterinarian", "dental hygienist", "dental assistant",
	"pharmacist", "phlebotomist", "paramedic", "medical assistant",
	"physical therapist", "occupational therapist", "massage therapist",
	// Skilled trades
	"electrician", "plumber", "welder", "carpenter", "machinist", "millwright",
	"forklift",
	// Hospitality & food service. Bare "chef" is deliberately absent — it collides
	// with Progress Chef (config-management), which would mislabel a DevOps/SRE
	// title the tech dictionary did not place; the cook terms cover food service.
	"barista", "bartender", "line cook", "prep cook", "dishwasher",
	"housekeeper", "housekeeping", "banquet", "waiter", "waitress", "busser",
	"concierge", "valet",
	// Retail & warehouse
	"cashier", "stocker", "merchandiser",
	// Personal care & fitness
	"pilates instructor", "yoga instructor", "fitness instructor",
	"personal trainer", "cosmetologist", "hair stylist", "hairstylist",
	"barber", "esthetician",
	// Education & childcare
	"teacher", "tutor", "preschool",
	// Facilities & cleaning
	"janitor", "janitorial", "cleaner", "custodian", "groundskeeper",
	// Security & transport
	"security guard", "truck driver", "delivery driver", "bus driver", "courier",
	// Front-of-house administration
	"receptionist", "front desk",
}

// IsNonTech reports whether a job title states a confidently non-technical role,
// matching any nonTechTitleTerms term on word boundaries. It never guesses: a
// title it cannot confidently place returns false. It resolves ONLY non-technical
// roles — a technical title yields false — so it can feed the is_tech derivation
// without risking a technical job being mislabelled.
func IsNonTech(title string) bool {
	lower := strings.ToLower(title)
	for _, term := range nonTechTitleTerms {
		if wordmatch.Contains(lower, term, wordmatch.UnicodeBoundary) {
			return true
		}
	}
	return false
}
