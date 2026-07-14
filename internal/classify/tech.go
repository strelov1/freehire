package classify

import (
	"strings"

	"github.com/strelov1/freehire/internal/wordmatch"
)

// techTitleTerms is a curated set of confidently technical (software/IT) role
// terms found in job titles, the positive counterpart to nonTechTitleTerms. It
// exists because is_tech's `true` was otherwise set only by a recognized tech
// CATEGORY, so generic software titles that resolve no sub-discipline ("Software
// Engineer", "Web3 Developer", "System Administrator") fell into `unknown`,
// undercounting tech.
//
// Same doctrine as the rest of classify: whole-word match, never guess. The
// governing rule is the "engineer"/"developer" trap: prod titles show bare
// "engineer" is dominated by NON-software roles (mechanical, manufacturing,
// civil, drainage, optical, project) and bare "developer" also names non-tech
// roles (business/real-estate developer). So every term here is SOFTWARE-ANCHORED
// — no bare "engineer"/"developer"/"analyst"/"architect"/"administrator" — and a
// non-software "…Engineer" stays `unknown` rather than being mislabelled tech.
var techTitleTerms = []string{
	// Software engineer forms (never bare "engineer")
	"software engineer", "software development engineer", "devops engineer",
	"site reliability engineer", "platform engineer",
	"backend engineer", "back-end engineer", "frontend engineer", "front-end engineer",
	"fullstack engineer", "full stack engineer", "full-stack engineer",
	"data engineer", "machine learning engineer", "ml engineer", "ai engineer",
	"cloud engineer", "security engineer", "qa engineer", "test engineer",
	"network engineer", "mobile engineer", "web engineer", "firmware engineer",
	"embedded engineer", "infrastructure engineer", "systems software engineer",
	// Developer forms (never bare "developer")
	"software developer", "web developer", "backend developer", "back-end developer",
	"frontend developer", "front-end developer", "fullstack developer",
	"full stack developer", "full-stack developer", "mobile developer",
	"app developer", "application developer", "game developer", "salesforce developer",
	"sharepoint developer", "web3 developer", "blockchain developer",
	"smart contract developer", "ios developer", "android developer",
	"python developer", "java developer", "javascript developer", "typescript developer",
	"golang developer", "go developer", ".net developer", "dotnet developer",
	"php developer", "ruby developer", "rails developer", "c# developer",
	"c++ developer", "node developer", "nodejs developer", "node.js developer",
	"react developer", "react native developer", "angular developer", "vue developer",
	"wordpress developer", "drupal developer", "magento developer", "shopify developer",
	"database developer", "etl developer", "bi developer", "power bi developer",
	"rpa developer", "erp developer", "sap developer", "oracle developer", "abap developer",
	// Administration / operations (never bare "administrator")
	"system administrator", "systems administrator", "sysadmin", "network administrator",
	"database administrator", "linux administrator", "windows administrator",
	"it administrator", "devsecops",
	// Architects (never bare "architect")
	"software architect", "solutions architect", "cloud architect", "data architect",
	"security architect", "enterprise architect", "technical architect",
	// Data / ML / security specialisms
	"data scientist", "machine learning", "deep learning", "computer vision engineer",
	"nlp engineer", "penetration tester", "pentester", "sdet", "software tester",
	// Unambiguous single words
	"programmer", "sre", "devops",
}

// IsTech reports whether a job title states a confidently technical (software/IT)
// role, matching any techTitleTerms term on word boundaries. It never guesses: a
// title it cannot confidently place as technical returns false. It resolves ONLY
// technical roles — a non-software "…Engineer" (mechanical, drainage, …) yields
// false — so it can feed the is_tech derivation as an additional `true` source
// without risking a non-tech job being mislabelled.
func IsTech(title string) bool {
	lower := strings.ToLower(title)
	for _, term := range techTitleTerms {
		if wordmatch.Contains(lower, term, wordmatch.UnicodeBoundary) {
			return true
		}
	}
	return false
}
