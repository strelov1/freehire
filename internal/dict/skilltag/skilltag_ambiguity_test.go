package skilltag

import (
	"slices"
	"testing"
)

// Ambiguous English words that double as tech names (react/swift/rust/spring/…)
// must only tag when the same text carries an unambiguous tech token
// (corroboration). This kills the dominant false-positive class — a non-tech post
// that merely says "must react to changes" or "strong networking skills" — while a
// real posting that also names its stack keeps the tag. Unambiguous alias forms
// (reactjs, "react native", "spring boot") always tag, no corroboration needed.
func TestParse_AmbiguousCorroboration(t *testing.T) {
	contains := func(hay []string, needle string) bool {
		for _, h := range hay {
			if h == needle {
				return true
			}
		}
		return false
	}
	cases := []struct {
		name   string
		in     string
		want   []string
		absent []string
	}{
		// bare ambiguous word, NO unambiguous tech token → dropped
		{"react verb (cook)", "Must be able to react to changes in schedules.", nil, []string{"react"}},
		{"networking soft skill", "Strong networking and negotiation skills.", nil, []string{"networking"}},
		{"spring season", "Spring and summer seasonal cleaning crew.", nil, []string{"spring"}},
		{"rust corrosion", "Inspect the railings for rust and repaint.", nil, []string{"rust"}},
		{"swift adjective", "We need a swift and friendly barista.", nil, []string{"swift"}},
		{"ruby name", "Report directly to Ruby, the floor manager.", nil, []string{"ruby"}},
		{"cloud weather", "Outdoor role, rain or cloud, year round.", nil, []string{"cloud"}},
		{"soap cleaning product", "Restock hand soap and paper towels in the restroom.", nil, []string{"soap"}},
		{"s3 roadmap phase", "We finished the S3 phase of the roadmap.", nil, []string{"s3"}},
		// broad concepts alone (non-tech context) → dropped
		{"ai marketing", "AI-powered marketing automation for our sales team.", nil, []string{"ai", "automation"}},
		{"crm sales role", "Manage our CRM as an account executive.", nil, []string{"crm"}},
		{"broad concepts don't self-corroborate", "AI, automation and analytics for retail.", nil, []string{"ai", "automation", "analytics"}},
		// broad concept + concrete tech → kept
		{"ai + pytorch", "AI models with PyTorch and Python.", []string{"ai", "pytorch", "python"}, nil},
		// corroborated by an unambiguous tech token → kept
		{"react + typescript", "React and TypeScript for our UI.", []string{"react", "typescript"}, nil},
		{"networking + linux", "Networking with Linux, BGP and firewalls.", []string{"networking", "linux", "bgp"}, nil},
		{"swift + ios", "Swift developer building for iOS.", []string{"swift", "ios"}, nil},
		{"cloud + aws", "Cloud infrastructure on AWS and Kubernetes.", []string{"cloud", "aws", "kubernetes"}, nil},
		{"soap + dotnet", "Maintain a legacy SOAP service built on .NET.", []string{"soap", "dotnet"}, nil},
		{"s3 + lambda", "Store uploads in S3 and process them with Lambda.", []string{"s3"}, nil},
		// unambiguous alias forms tag WITHOUT corroboration
		{"reactjs standalone", "We build with reactjs.", []string{"react"}, nil},
		{"react native phrase", "React Native mobile apps.", []string{"react-native", "react"}, nil},
		{"spring boot phrase", "Spring Boot services.", []string{"spring"}, nil},
		{"ruby on rails phrase", "A Ruby on Rails shop.", []string{"ruby", "rails"}, nil},
		{"soap api phrase", "We expose a SOAP API for legacy partners.", []string{"soap"}, nil},
		{"aws s3 phrase", "Upload files to AWS S3 for storage.", []string{"s3"}, nil},
		{"amazon s3 phrase", "Backups are stored in Amazon S3.", []string{"s3"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Parse(c.in)
			for _, w := range c.want {
				if !contains(got, w) {
					t.Errorf("Parse(%q) = %v, missing %q", c.in, got, w)
				}
			}
			for _, a := range c.absent {
				if contains(got, a) {
					t.Errorf("Parse(%q) = %v, must NOT contain %q", c.in, got, a)
				}
			}
		})
	}
}

// The framework names react/swift/rust/spring/express are proper nouns — "React",
// "Swift" — while the English words they collide with are written in lowercase in the
// middle of a sentence: "ability to react quickly", "a swift turnaround", "express
// interest". Corroboration alone cannot tell them apart, because a non-technical
// posting still names tools (Zendesk, HubSpot) or a broad phrase (machine learning)
// that lifts the gate. Both prod cases below were tagged react before the fix
// (2026-10-29): a client support specialist and a data director.
func TestParse_ProperNounWordsNeedACapital(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   []string
		absent []string
	}{
		{"react verb beside support tools", "Ability to react quickly. We use Zendesk and HubSpot.",
			[]string{"hubspot", "zendesk"}, []string{"react"}},
		{"react verb beside machine learning", "shape priorities rather than react to decisions. analytics, machine learning",
			[]string{"machine-learning"}, []string{"react"}},
		{"client support specialist (Mapon)", "Client Support Specialist. Ability to react quickly and concentrate in stressful situations. Knowledge of Salesforce.",
			[]string{"salesforce"}, []string{"react"}},
		{"swift adjective beside a tech token", "Ensure a swift turnaround on tickets in Jira and Salesforce.",
			[]string{"jira", "salesforce"}, []string{"swift"}},
		{"express verb beside a tech token", "To express interest, apply here. Our backend runs on Python.",
			[]string{"python"}, []string{"express"}},
		{"spring season beside a tech token", "Complete your PhD by spring 2027. Python and SQL required.",
			[]string{"python", "sql"}, []string{"spring"}},
		{"rust corrosion beside a tech token", "Check the pumps for rust; log findings in SAP and Excel via Python.",
			[]string{"python"}, []string{"rust"}},

		{"react + typescript", "Experience with React and TypeScript", []string{"react", "typescript"}, nil},
		{"react list", "React, Redux, Node.js", []string{"react", "redux", "nodejs"}, nil},
		{"react all caps", "SENIOR REACT DEVELOPER, TYPESCRIPT", []string{"react", "typescript"}, nil},
		{"capital beats a lowercase verb elsewhere", "We react fast to feedback. Stack: React and TypeScript.",
			[]string{"react", "typescript"}, nil},
		{"swift + ios", "Swift developer building for iOS.", []string{"swift", "ios"}, nil},
		{"spring + java", "Java 17 with Spring and Hibernate.", []string{"spring", "java"}, nil},
		{"express + nodejs", "APIs in Express on Node.js.", []string{"express", "nodejs"}, nil},
		{"rust + kubernetes", "Services in Rust deployed on Kubernetes.", []string{"rust", "kubernetes"}, nil},
		{"lowercase unambiguous forms stay strong", "we build with reactjs and react.js", []string{"react"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Parse(c.in)
			for _, w := range c.want {
				if !slices.Contains(got, w) {
					t.Errorf("Parse(%q) = %v, missing %q", c.in, got, w)
				}
			}
			for _, a := range c.absent {
				if slices.Contains(got, a) {
					t.Errorf("Parse(%q) = %v, must NOT contain %q", c.in, got, a)
				}
			}
		})
	}
}

// A skill list carries no English verb for "react" to be, so WithSkillList keeps a
// lowercase proper-noun word; corroboration still applies.
func TestParse_WithSkillListKeepsLowercaseProperNouns(t *testing.T) {
	if got := Parse("react typescript spring java", WithSkillList()); !slices.Equal(got, []string{"java", "react", "spring", "typescript"}) {
		t.Errorf("Parse(list, WithSkillList()) = %v, want [java react spring typescript]", got)
	}
	if got := Parse("react typescript"); slices.Contains(got, "react") {
		t.Errorf("Parse(prose) = %v, must NOT contain react", got)
	}
	if got := Parse("react", WithSkillList()); got != nil {
		t.Errorf("Parse(lone ambiguous word, WithSkillList()) = %v, want nil (still needs corroboration)", got)
	}
}

// Job-posting boilerplate that happens to spell a tech name. Unlike the collisions
// above — which read as ordinary English anywhere — these are the words that recur in
// the prose of NON-TECHNICAL postings specifically: an "agile" retail supervisor, the
// drivers who are "the backbone" of a haulier, a "restful" night in a care home, the
// factory "assembly" line, the rugby "scrum", tree "sap" on a guard rail, a "sentry"
// post, a bank "vault", the fire-code "firewall", a "rancher", a "postman", "braze"d
// copper joints.
//
// Each was a strong alias, so one of them alone tagged the posting — and worse, a
// strong match lifts the corroboration gate off EVERY weak word in the same text, so a
// single boilerplate hit also legitimises the react/rust/spring collisions beside it.
// That is how a Highway Maintenance Supervisor came out of the pipeline tagged
// {react, sap}. Measured on prod, 20855 boards holding 475625 postings were kept off
// the retirement report by the skills dictionary alone, with no technical posting
// anywhere.
//
// Gating them costs nothing a real posting needs: a genuine SAP role names ABAP, a
// REST role says "REST API" (a strong phrase), a security role names its stack.
func TestParse_BoilerplateWordsNeedCorroboration(t *testing.T) {
	contains := func(hay []string, needle string) bool {
		for _, h := range hay {
			if h == needle {
				return true
			}
		}
		return false
	}
	cases := []struct {
		name   string
		in     string
		want   []string
		absent []string
	}{
		// Boilerplate in non-technical prose, no other tech token → dropped.
		{"agile as a temperament", "We want an agile, hands-on retail supervisor.", nil, []string{"agile"}},
		{"backbone of the team", "Our drivers are the backbone of this haulage firm.", nil, []string{"backbone"}},
		{"restful care home", "Help residents enjoy a restful night in our care home.", nil, []string{"rest"}},
		{"assembly line", "Work the assembly line fitting door panels.", nil, []string{"assembly"}},
		{"rugby scrum", "Coach the scrum and play tight-head prop.", nil, []string{"scrum"}},
		{"tree sap", "Clear vegetation and wash tree sap off the guard rails.", nil, []string{"sap"}},
		{"sentry duty", "Stand sentry at the north gate on night shifts.", nil, []string{"sentry"}},
		{"bank vault", "Count the drawer and lock the vault at close.", nil, []string{"vault"}},
		{"fire-code firewall", "Inspect the firewall between the garage and the dwelling.", nil, []string{"firewall"}},
		{"ranch hand", "Assist the rancher with calving and fence repair.", nil, []string{"rancher"}},
		{"postal round", "Cover the postman's round while they are on leave.", nil, []string{"postman"}},
		{"brazing copper", "Braze copper joints on refrigeration units.", nil, []string{"braze"}},
		// Corroborated by an unambiguous tech token → kept, exactly as before.
		{"agile + jira", "Agile delivery tracked in Jira and Kubernetes.", []string{"agile", "jira", "kubernetes"}, nil},
		{"sap + abap", "SAP ABAP developer for our finance modules.", []string{"sap", "abap"}, nil},
		{"assembly + c", "Assembly and C programming for embedded targets.", []string{"assembly", "c"}, nil},
		{"firewall + linux", "Configure firewalls and BGP on Linux.", []string{"firewall", "bgp", "linux"}, nil},
		{"sentry + typescript", "Error tracking with Sentry in a TypeScript app.", []string{"sentry", "typescript"}, nil},
		{"vault + kubernetes", "HashiCorp Vault secrets on Kubernetes.", []string{"vault", "kubernetes"}, nil},
		{"scrum + jira", "Scrum ceremonies tracked in Jira.", []string{"scrum", "jira"}, nil},
		{"rancher + kubernetes", "Rancher-managed Kubernetes clusters.", []string{"rancher", "kubernetes"}, nil},
		{"backbone + jquery", "A legacy Backbone and jQuery front end.", []string{"backbone", "jquery"}, nil},
		{"braze + typescript", "Braze lifecycle campaigns instrumented in TypeScript.", []string{"braze", "typescript"}, nil},
		// The unambiguous phrase form still tags on its own — "REST API" is not prose.
		{"rest api phrase", "Design REST APIs in Python.", []string{"rest", "python"}, nil},
		{"postman + rest api", "API testing with Postman against our REST APIs.", []string{"postman", "rest"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Parse(c.in)
			for _, w := range c.want {
				if !contains(got, w) {
					t.Errorf("Parse(%q) = %v, missing %q", c.in, got, w)
				}
			}
			for _, a := range c.absent {
				if contains(got, a) {
					t.Errorf("Parse(%q) = %v, must NOT contain %q", c.in, got, a)
				}
			}
		})
	}
}

// Every ambiguousWords key must be a real wordAliases entry — an ambiguous marker
// on a token the word pass never emits would be dead config.
func TestAmbiguousWordsSubsetOfAliases(t *testing.T) {
	for w := range ambiguousWords {
		if _, ok := wordAliases[w]; !ok {
			t.Errorf("ambiguousWords[%q] has no wordAliases entry", w)
		}
	}
}
