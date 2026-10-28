package classify

import "testing"

// TestFrenchHeadsAndFields pins the French title vocabulary. A French title names a
// head and the field it works in ("Technicien de maintenance", "Conseiller de vente"),
// so a role noun or a field resolves and a bare generic head does not. Every title here
// is a live posting from the French-language crawl (wttj, flatchr, werecruit,
// smartrecruiters, teamtailor, workday, eures, recruitee, lever, join), measured
// 2026-10-28.
func TestFrenchHeadsAndFields(t *testing.T) {
	cases := []struct {
		title, category, seniority string
	}{
		// The gender forms: the notice, the parenthesis, the point médian, the dot and
		// the slash, and the feminine word.
		{"Développeur(se) Java F/H", "software_engineering", ""},
		{"Développeur.se iOS - Lille", "mobile", ""},
		{"Ingénieur·e Validation Logicielle", "qa", ""},
		{"Technicien/ne de maintenance", "skilled_trades", ""},
		{"Conseiller/ère de Vente (H/F) - CDD Temps-Plein", "sales", ""},
		{"Assistante de vie (H/F)", "healthcare", ""},
		{"Infirmière de nuit H/F", "healthcare", ""},
		{"Chargée administration des ventes H/F", "administration", ""},
		{"ADMINISTRATEUR/RICE SYSTEMES ET RESEAUX (H/F)", "devops", ""},
		{"SECRETAIRE MEDICAL(E) CONFIRME(E) EN CABINET D’OPHTALMOLOGIE (H/F)", "administration", "middle"},
		{"Vendeuse en Lingerie Itinérante H/F", "sales", ""},
		// The main table already carries the alias; only the notice hid it.
		{"Ingénieur·e DevOps", "devops", ""},

		// A head takes its field's category.
		{"Gestionnaire de paie confirmé H/F/X", "finance", "middle"},
		{"Assistant Ressources Humaines en alternance F/H", "hr", "intern"},
		{"Chargé de recrutement F/H", "recruiting", ""},
		{"Chargé de communication régional F/H", "marketing", ""},
		{"Agent de sécurité H/F", "public_safety", ""},
		{"Agent d'entretien polyvalent H/F", "personal_services", ""},
		{"Opérateur de production H/F", "manufacturing", ""},
		{"Préparateur de commandes Magasinier H/F", "logistics", ""},
		{"Employé Polyvalent de Restauration et de Petit-Déjeuner (F/H) - CDI", "hospitality", ""},
		{"Auxiliaire de crèche en alternance", "education", "intern"},
		{"Educateur spécialisé (H/F)", "social_services", ""},
		{"Technicien informatique F/H", "it_support", ""},
		{"Ingénieur Cloud GCP (H/F)", "devops", ""},
		{"Architecte Data (F/H)", "architecture", ""},
		{"INGENIEUR CYBERSECURITE F/H", "security", ""},
		{"Ingénieur Méthodes Electrique – Toulouse H/F", "industrial_engineering", ""},
		{"Dessinateur projeteur SolidWorks H/F", "engineering_design", ""},
		{"Consultant Fonctionnel ERP H/F", "business_analysis", ""},
		{"Animateur QSE H/F", "occupational_safety", ""},
		// Québec's grocery clerk is headed by the department.
		{"Épicerie Commis temps partiel nuit", "retail", ""},
		// Belgium's "technicien de surface" cleans.
		{"TECHNICIEN DE SURFACE - ANDENNE (H/F/X)", "personal_services", ""},

		// A head and field whose answer is not the field's.
		{"Développeur commercial H/F", "sales", ""},
		{"Employé Commercial H/F Rayon Frais Libre Service", "retail", ""},
		{"Ingénieur commercial - Région parisienne (H/F)", "sales", ""},
		{"Technicien de maintenance informatique H/F", "it_support", ""},
		{"Ingénieur d’Application – Métrologie Semi-conducteurs F/H", "hardware", ""},
		{"Monteur Vidéo Freelance UGC & VSL", "creative", ""},
		{"Commercial en assurance H/F", "sales", ""},

		// French "chef de X" leads any desk; only the kitchen's chef cooks.
		{"Chef de projet IT", "project_management", ""},
		{"Chef.fe de projet delivery - Défense & Sécurité – Lille", "project_management", ""},
		{"Chef de produit DATA H/F", "product", ""},
		{"Chef de chantier - Electricité", "skilled_trades", ""},
		{"CHEF  DE RAYON (H/F)", "retail", ""},
		{"Chef d'équipe logistique H/F", "logistics", "lead"},
		{"Chef d&#39;équipe Monteur Frigoriste", "skilled_trades", "lead"},
		{"Chef de partie H/F", "hospitality", ""},
		{"Chef de cuisine (H/F)", "hospitality", ""},
		{"Sous-chef (H/F)", "hospitality", ""},
		// The French electrician's "CFO" is courants forts, not the executive.
		{"Technicien de maintenance CFO/CFA F/H", "skilled_trades", ""},
		{"Conducteur de travaux CFO F/H", "skilled_trades", ""},
		{"Electricien Cfo / Cfa H/f (H/F)", "skilled_trades", ""},
		{"Électricien CFO H/F", "skilled_trades", ""},
		{"Chargé d'Études CFO", "skilled_trades", ""},
		{"CFO", "finance", ""},

		// A workplace is the client's sector on a technical title.
		{"Tech Lead JAVA/J2EE - Transport, secteur public F/H", "", "lead"},
		{"Conseiller clientèle banque H/F", "support", ""},

		// A generic head with no field resolves nothing.
		{"Responsable d'exploitation H/F", "", ""},
		{"Technicien itinérant H/F", "", ""},
		{"Ingénieur H/F", "", ""},
		{"Gestionnaire de propriétés - Rejoignez le 1er réseau de conciergerie", "", ""},

		// Grades.
		{"Stage - Chef de Projet Influence", "project_management", "intern"},
		{"Stagiaire développeur android", "mobile", "intern"},
		{"Apprenti(e) cuisinier CFC", "hospitality", "intern"},
		{"Data Engineer Confirmé", "data_engineering", "middle"},
		{"Commercial débutant futur cadre manager (39) H/F", "sales", "junior"},
		{"Développeur Java sénior (F/H)", "software_engineering", "senior"},
		{"DIRECTEUR GENERAL INTERIMAIRE (H/F)", "", "c_level"},
		{"Directeur Administratif et Financier Futur Associé H/F", "finance", "c_level"},
		// The assistant of an executive and the desk that runs the trainee programmes
		// hold no grade of their own.
		{"Assistante du Directeur Général H/F", "", ""},
		{"Responsable alternance et relations écoles H/F", "", ""},
		// "Chef de projet" is a desk, not a grade, and "directeur" alone is no board.
		{"Chef de projet H/F", "project_management", ""},
		{"Directeur de magasin H/F", "retail", ""},
	}
	for _, c := range cases {
		got := Parse(c.title)
		if got.Category != c.category || got.Seniority != c.seniority {
			t.Errorf("Parse(%q) = {%q, %q}, want {%q, %q}", c.title, got.Category, got.Seniority, c.category, c.seniority)
		}
	}
}

// TestFrenchReadsOnlyFrenchTitles pins the guard that keeps the French table off English
// titles: its words "commercial", "assistant", "agent", "maintenance", "stage" are English
// ones too, and an English title the main table left unresolved stays unresolved.
func TestFrenchReadsOnlyFrenchTitles(t *testing.T) {
	cases := []struct {
		title, category, seniority string
	}{
		{"Commercial Associate", "", ""},
		{"Boutique Associate", "", ""},
		{"Maintenance Planner", "", ""},
		{"Programmer Analyst, System and infrastructure operations, maintenance and development", "", ""},
		{"Account Executive - Early Stage", "sales", ""},
		{"Stage Manager", "", ""},
		{"Maintenance Planner • Austin", "", ""},
		{"Hotel Restaurant Supervisor", "", ""},
	}
	for _, c := range cases {
		got := Parse(c.title)
		if got.Category != c.category || got.Seniority != c.seniority {
			t.Errorf("Parse(%q) = {%q, %q}, want {%q, %q}", c.title, got.Category, got.Seniority, c.category, c.seniority)
		}
	}
}

// TestFrenchAliasesAreFolded holds every French alias to the space foldFrench maps a
// title into: an alias the fold would rewrite could never match.
func TestFrenchAliasesAreFolded(t *testing.T) {
	for _, table := range [][]aliasEntry{frenchCategoryTable, frenchWorkplaceTable, frenchSeniorityTable} {
		for _, e := range table {
			if f := foldFrench(e.alias); f != e.alias {
				t.Errorf("alias %q folds to %q", e.alias, f)
			}
		}
	}
}
