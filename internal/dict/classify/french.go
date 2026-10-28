package classify

import (
	"regexp"
	"strings"
)

// French titles share the Spanish shape — a HEAD followed by the FIELD it works in
// ("Technicien de maintenance", "Conseiller de vente", "Chargé de recrutement",
// "Agent de quai") — but almost none of their words are the Spanish ones, so the
// Spanish and Portuguese fallbacks read next to nothing of them. Measured 2026-10-28
// over 24,609 open French-language postings (wttj, flatchr, werecruit, smartrecruiters,
// teamtailor, workday, eures, recruitee, lever, join, workable, greenhouse, personio,
// read per source with `countries @> {fr}` where the source spans countries), 80% of
// the titles resolved no category.
//
// The table is ordered in the Spanish bands, and table order is the precedence:
//
//  1. a head and field whose answer differs from the field's default ("Développeur
//     commercial" sells, "Employé commercial" stocks a supermarket's shelves,
//     "Assistant de vie" cares for someone);
//  2. the role nouns that name a trade by themselves (infirmier, cariste, serveur,
//     électricien, comptable);
//  3. the fields themselves (maintenance, logistique, restauration, paie), which give
//     a generic head its category;
//  4. the generic heads that name a desk on their own (commercial, secrétaire).
//
// A generic head with no field — technicien, assistant, agent, chargé, responsable,
// gestionnaire, conseiller, consultant, ingénieur, directeur, employé, opérateur —
// resolves nothing, as "Specialist" and "Manager" resolve nothing in the main table.
//
// Every alias is written in the space foldFrench maps a title into: lowercased,
// unaccented, every gender notice cut ("(H/F)", "F/H", "(e)", "·ne", "/euse") and the
// feminine words folded onto their masculine ("développeuse" → "developpeur",
// "conseillère" → "conseiller", "chargée" → "charge"). A test holds every alias to
// that space, so an alias the fold would rewrite cannot sit in the table unreachable.
//
// The table is a FALLBACK: it runs only after the main, German, Dutch, Danish, Spanish
// and Portuguese tables resolved nothing, and only on a title that looks French
// (looksFrench), because many of its words are English ones as well — "commercial",
// "assistant", "agent", "boutique", "restaurant" — and an English title the main table
// left unresolved must not be read as French. Before it, the main table is asked once
// more of the gender-cut title: "Data Analyst (F/H)" reads fine, but "Développeur.se
// Full Stack" and "Ingénieur·e DevOps" hide an alias behind the notice.
var frenchCategoryTable = []aliasEntry{
	// Band 1: a head and field whose answer is not the field's.

	// The business, property and network developer, before the software one: French
	// "développeur" names all of them.
	{"developpeur commercial", "sales"},
	{"developpeur d'affaires", "sales"},
	{"developpeur business", "sales"},
	{"developpement commercial", "sales"},
	{"developpement des ventes", "sales"},
	{"developpeur immobilier", categoryNone},
	{"developpeur foncier", categoryNone},
	{"developpement foncier", categoryNone},
	{"developpeur territorial", categoryNone},
	{"developpeur economique", categoryNone},
	{"developpeur de reseau", categoryNone}, // a franchise network's developer
	{"developpement de reseau", categoryNone},
	{"developpeur produit", categoryNone}, // a fashion or food product developer
	{"developpeur de produits", categoryNone},
	{"developpement rh", "hr"},
	{"developpement des competences", "hr"},
	// The semiconductor is not a driver ("Métrologie semi-conducteurs").
	{"semi-conducteur", "hardware"},
	{"semi-conducteurs", "hardware"},
	{"semiconducteur", "hardware"},
	{"semiconducteurs", "hardware"},
	// Radio communication is a network, not a communications desk.
	{"radiocommunication", "network_engineering"},
	{"radiocommunications", "network_engineering"},
	{"radio communication", "network_engineering"},
	// Industrial computing is the plant's automation, not the help desk.
	{"informatique industriel", "industrial_engineering"},
	{"informatique de gestion", "software_engineering"},
	// The machine shop, before the developer and the programmer.
	{"programmeur cn", "manufacturing"},
	{"programmeur cnc", "manufacturing"},
	{"programmeur commande numerique", "manufacturing"},
	{"programmeur fao", "manufacturing"},
	{"programmeur automate", categoryNone}, // a plant's PLC programmer
	{"programmeur automaticien", categoryNone},
	{"programmeur robot", categoryNone},

	// Information security and the guard, before "sécurité" alone.
	{"cybersecurite", "security"},
	{"cyber securite", "security"},
	{"cyberdefense", "security"},
	{"securite reseau", "security"},
	{"securite reseaux", "security"},
	{"securite informatique", "security"},
	{"securite des systemes d'information", "security"},
	{"securite du systeme d'information", "security"},
	{"securite de l'information", "security"},
	{"securite des si", "security"},
	{"securite si", "security"},
	{"securite operationnel", "security"},
	{"rssi", "security"},
	{"analyste soc", "security"},
	{"pentesteur", "security"},
	{"auditeur securite", "security"},
	// Occupational safety, before the guard.
	{"qhse", "occupational_safety"},
	{"qse", "occupational_safety"},
	{"hse", "occupational_safety"},
	{"hseq", "occupational_safety"},
	{"sante securite", "occupational_safety"},
	{"sante et securite", "occupational_safety"},
	{"securite au travail", "occupational_safety"},
	{"prevention des risques", "occupational_safety"},
	{"preventeur", "occupational_safety"},
	{"securite environnement", "occupational_safety"},
	// Fire safety systems are a trade; the fire-safety officer (SSIAP) is a guard.
	{"technicien ssi", "skilled_trades"},
	{"maintenance ssi", "skilled_trades"},
	{"securite incendie", "public_safety"},
	{"ssiap", "public_safety"},
	{"agent de securite", "public_safety"},
	{"agent de surete", "public_safety"},
	{"agent de prevention et de securite", "public_safety"},
	{"agent de surveillance", "public_safety"},
	{"agent cynophile", "public_safety"},
	{"agent de gardiennage", "public_safety"},
	{"gardiennage", "public_safety"},
	{"maitre-chien", "public_safety"},
	{"maitre chien", "public_safety"},
	{"surete aeroportuaire", "public_safety"},
	{"police municipal", "public_safety"},
	{"sapeur-pompier", "public_safety"},
	{"sapeur pompier", "public_safety"},
	{"pompier", "public_safety"},

	// IT.
	{"ingenieur logiciel", "software_engineering"},
	{"ingenieur developpement", "software_engineering"},
	{"ingenieur de developpement", "software_engineering"},
	{"ingenieur etudes et developpement", "software_engineering"},
	{"ingenieur d'etudes et developpement", "software_engineering"},
	{"concepteur developpeur", "software_engineering"},
	{"concepteur-developpeur", "software_engineering"},
	{"analyste developpeur", "software_engineering"},
	{"analyste programmeur", "software_engineering"},
	{"developpeur", "software_engineering"},
	{"developpeurs", "software_engineering"},
	{"programmeur", "software_engineering"},
	{"developpement logiciel", "software_engineering"},
	{"developpement web", "software_engineering"},
	{"developpement informatique", "software_engineering"},
	{"ingenieur informatique", "software_engineering"},
	{"informaticien", "software_engineering"},
	{"ingenieur data", "data_engineering"},
	{"ingenieur donnees", "data_engineering"},
	{"ingenieur de donnees", "data_engineering"},
	{"ingenieur big data", "data_engineering"},
	{"analyste de donnees", "data_analytics"},
	{"analyste donnees", "data_analytics"},
	{"analyste data", "data_analytics"},
	{"analyste decisionnel", "data_analytics"},
	{"consultant decisionnel", "data_analytics"},
	{"developpeur decisionnel", "data_analytics"},
	{"informatique decisionnel", "data_analytics"},
	{"scientifique des donnees", "data_science"},
	{"ingenieur ia", "ai_engineering"},
	{"ingenieur en ia", "ai_engineering"},
	{"ingenieur intelligence artificiel", "ai_engineering"},
	{"ingenieur en intelligence artificiel", "ai_engineering"},
	{"ia generative", "ai_engineering"},
	{"apprentissage automatique", "ml_ai"},
	{"analyste fonctionnel", "business_analysis"},
	{"consultant fonctionnel", "business_analysis"},
	{"analyste metier", "business_analysis"},
	{"testeur", "qa"},
	{"ingenieur test", "qa"},
	{"ingenieur tests", "qa"},
	{"ingenieur de test", "qa"},
	{"ingenieur recette", "qa"},
	{"testeur recette", "qa"},
	{"recette fonctionnel", "qa"},
	{"test logiciel", "qa"},
	{"tests logiciels", "qa"},
	{"qualite logiciel", "qa"},
	{"validation logiciel", "qa"},
	{"dba", "devops"},
	{"ingenieur systeme linux", "devops"},
	{"ingenieur systemes linux", "devops"},
	{"administrateur systeme", "devops"}, // the main table files the system administrator here
	{"administrateur systemes", "devops"},
	{"ingenieur systeme et reseau", "devops"},
	{"ingenieur systemes et reseaux", "devops"},
	{"ingenieur cloud", "devops"},
	{"ingenieur infrastructure", "devops"},
	{"ingenieur de production informatique", "devops"},
	{"ingenieur d'exploitation", "devops"},
	{"administrateur de base de donnees", "devops"},
	{"administrateur de bases de donnees", "devops"},
	{"administrateur bases de donnees", "devops"},
	{"administrateur reseau", "network_engineering"},
	{"administrateur reseaux", "network_engineering"},
	{"ingenieur reseau", "network_engineering"},
	{"ingenieur reseaux", "network_engineering"},
	{"ingenieur telecom", "network_engineering"},
	{"ingenieur telecoms", "network_engineering"},
	{"technicien reseau", "network_engineering"},
	{"technicien reseaux", "network_engineering"},
	{"technicien informatique", "it_support"},
	{"technicien support", "it_support"},
	{"technicien helpdesk", "it_support"},
	{"technicien de proximite", "it_support"},
	{"technicien poste de travail", "it_support"},
	{"technicien systeme", "it_support"},
	{"technicien systemes", "it_support"},
	{"exploitation informatique", "it_support"},
	{"support informatique", "it_support"},
	{"support utilisateur", "it_support"},
	{"support utilisateurs", "it_support"},
	{"assistance informatique", "it_support"},
	{"ingenieur support", "it_support"},
	{"ingenieur systeme embarque", "embedded"},
	{"ingenieur systemes embarques", "embedded"},
	{"systemes embarques", "embedded"},
	{"electronique embarque", "hardware"},
	{"ingenieur electronique", "hardware"},
	{"ingenieur en electronique", "hardware"},
	{"conception electronique", "hardware"},
	{"cao electronique", "hardware"},
	{"technicien electronique", "hardware"},
	{"architecte logiciel", "architecture"},
	{"architecte solution", "architecture"},
	{"architecte solutions", "architecture"},
	{"architecte cloud", "architecture"},
	{"architecte data", "architecture"},
	{"architecte donnees", "architecture"},
	{"architecte si", "architecture"},
	{"architecte applicatif", "architecture"},
	{"architecte technique", "architecture"},
	{"architecte d'entreprise", "architecture"},
	{"architecte entreprise", "architecture"},
	{"architecte infrastructure", "architecture"},
	{"architecte systeme", "architecture"},
	{"architecte systemes", "architecture"},
	{"architecte reseau", "architecture"},
	{"architecte reseaux", "architecture"},
	{"architecte cybersecurite", "architecture"},
	{"architecte securite", "architecture"},
	{"architecte fonctionnel", "architecture"},
	{"architecte it", "architecture"},
	{"architecte d'interieur", "design"},
	{"architecte interieur", "design"},
	{"architecte paysagiste", "design"},
	{"consultant sap", "solutions_engineering"},
	{"ingenieur avant-vente", "solutions_engineering"},
	{"ingenieur avant vente", "solutions_engineering"},
	{"avant-vente", "solutions_engineering"},
	{"avant vente", "solutions_engineering"},

	// The engineer and the drafting office.
	{"ingenieur d'affaires", "sales"},
	{"ingenieur commercial", "sales"},
	{"ingenieur technico-commercial", "sales"},
	{"ingenieur methodes", "industrial_engineering"},
	{"ingenieur methode", "industrial_engineering"},
	{"ingenieur procedes", "industrial_engineering"},
	{"ingenieur procede", "industrial_engineering"},
	{"ingenieur process", "industrial_engineering"},
	{"ingenieur production", "industrial_engineering"},
	{"ingenieur de production", "industrial_engineering"},
	{"ingenieur qualite", "industrial_engineering"},
	{"ingenieur industrialisation", "industrial_engineering"},
	{"ingenieur maintenance", "industrial_engineering"},
	{"ingenieur de maintenance", "industrial_engineering"},
	{"ingenieur mecanique", "industrial_engineering"},
	{"ingenieur mecanicien", "industrial_engineering"},
	{"ingenieur electrique", "industrial_engineering"},
	{"ingenieur electricite", "industrial_engineering"},
	{"ingenieur electrotechnique", "industrial_engineering"},
	{"ingenieur genie civil", "industrial_engineering"},
	{"ingenieur genie electrique", "industrial_engineering"},
	{"ingenieur structure", "industrial_engineering"},
	{"ingenieur structures", "industrial_engineering"},
	{"ingenieur travaux", "industrial_engineering"},
	{"ingenieur essais", "industrial_engineering"},
	{"ingenieur materiaux", "industrial_engineering"},
	{"ingenieur chimiste", "industrial_engineering"},
	{"ingenieur automaticien", "industrial_engineering"},
	{"ingenieur automatisme", "industrial_engineering"},
	{"ingenieur cvc", "industrial_engineering"},
	{"ingenieur fluides", "industrial_engineering"},
	{"ingenieur thermique", "industrial_engineering"},
	{"ingenieur nucleaire", "industrial_engineering"},
	{"ingenieur aeronautique", "industrial_engineering"},
	{"ingenieur hydraulique", "industrial_engineering"},
	{"ingenieur conception mecanique", "engineering_design"},
	{"conception mecanique", "engineering_design"},
	{"dessinateur", "engineering_design"},
	{"projeteur", "engineering_design"},
	{"dessinateur-projeteur", "engineering_design"},
	{"bureau d'etudes", "engineering_design"},
	{"bureau d'etude", "engineering_design"},
	{"modeleur bim", "engineering_design"},
	{"projeteur bim", "engineering_design"},
	{"genie climatique", "skilled_trades"},
	{"methodes industrialisation", "industrial_engineering"},
	{"technicien methodes", "industrial_engineering"},
	{"automaticien", "industrial_engineering"},

	// Project and product.
	{"chef de projet", "project_management"},
	{"chef de projets", "project_management"},
	{"directeur de projet", "project_management"},
	{"directeur de projets", "project_management"},
	{"responsable de projet", "project_management"},
	{"responsable de projets", "project_management"},
	{"gestionnaire de projet", "project_management"},
	{"gestionnaire de projets", "project_management"},
	{"coordinateur de projet", "project_management"},
	{"coordinateur de projets", "project_management"},
	{"pilote de projet", "project_management"},
	{"pmo", "project_management"},
	{"chef de produit marketing", "marketing"},
	{"chef de produit", "product"},
	{"chef de produits", "product"},
	{"responsable produit", "product"},
	{"chef de chantier", "skilled_trades"},
	{"conducteur de travaux", "skilled_trades"},
	{"chef de rayon", "retail"},
	{"chef de caisse", "retail"},
	{"chef des ventes", "sales"},
	{"chef de quai", "logistics"},
	{"chef de reception", "hospitality"}, // the hotel's front desk

	// The sales desk whose answer is not "commercial"'s.
	{"employe commercial", "retail"}, // a supermarket's shelf stocker
	{"employe libre service", "retail"},
	{"employe libre-service", "retail"},
	{"employe de rayon", "retail"},
	// Switzerland's "employé de commerce CFC" is the office apprenticeship, not a shop.
	{"employe de commerce", "administration"},
	{"assistant commercial", "sales"},
	{"assistant adv", "administration"},
	{"administration des ventes", "administration"},
	{"gestionnaire adv", "administration"},
	{"charge d'affaires", "sales"},
	{"charge de developpement", "sales"},
	{"attache commercial", "sales"},
	{"delegue commercial", "sales"},
	{"delegue pharmaceutique", "sales"},
	{"delegue medical", "sales"},
	{"visiteur medical", "sales"},
	{"animateur commercial", "sales"},
	{"animateur des ventes", "sales"},
	{"agent commercial", "sales"},
	{"agent general", "sales"}, // the insurer's tied agent
	{"agent immobilier", "sales"},
	{"mandataire immobilier", "sales"},
	{"conseiller immobilier", "sales"},
	{"negociateur immobilier", "sales"},
	{"commercial immobilier", "sales"},
	{"conseiller de vente", "sales"},
	{"conseiller vente", "sales"},
	{"conseiller commercial", "sales"},
	{"conseiller en assurance", "sales"},
	{"conseiller en assurances", "sales"},
	{"conseiller en gestion de patrimoine", "finance"},
	{"conseiller patrimonial", "finance"},
	{"gestion de patrimoine", "finance"},
	{"conseiller bancaire", "finance"},
	{"conseiller clientele bancaire", "finance"},
	{"conseiller financier", "finance"},
	{"conseiller en insertion", "social_services"},
	{"conseiller en economie social", "social_services"},
	{"conseiller principal d'education", "education"},
	{"conseiller clientele", "support"},
	{"conseiller client", "support"},
	{"conseiller service client", "support"},
	{"teleconseiller", "support"},
	{"charge de clientele", "support"},
	{"charge de relation client", "support"},
	{"charge de relation clients", "support"},
	{"relation client", "support"},
	{"relation clients", "support"},
	{"relation clientele", "support"},
	{"service client", "support"},
	{"service a la clientele", "support"}, // Québec
	{"service clients", "support"},
	{"service apres-vente", "support"},
	{"apres-vente", "support"},
	{"sav", "support"},

	// Care: the carer, the nursery and the vet, before the generic assistant.
	{"assistant de vie", "healthcare"},
	{"auxiliaire de vie", "healthcare"},
	{"aide a domicile", "healthcare"},
	{"aide aux personnes agees", "healthcare"},
	{"aide-soignant", "healthcare"},
	{"aide soignant", "healthcare"},
	{"accompagnant educatif et social", "healthcare"},
	{"aide medico-psychologique", "healthcare"},
	{"agent de service hospitalier", "healthcare"},
	{"agent des services hospitaliers", "healthcare"},
	{"agent hospitalier", "healthcare"},
	{"infirmier auxiliaire", "healthcare"}, // Québec's practical nurse
	{"prepose aux beneficiaires", "healthcare"},
	{"auxiliaire veterinaire", "healthcare"},
	{"assistant veterinaire", "healthcare"},
	{"assistant dentaire", "healthcare"},
	{"assistant medical", "healthcare"},
	{"secretaire medical", "administration"}, // the main table's "medical secretary"
	{"assistant medico-administratif", "administration"},
	{"assistant maternel", "education"},
	{"auxiliaire de creche", "education"},
	{"auxiliaire de puericulture", "education"},
	{"agent de puericulture", "education"},
	{"auxiliaire petite enfance", "education"},
	{"petite enfance", "education"},
	{"educateur de jeunes enfants", "education"},
	{"accompagnant d'eleves", "education"},
	{"aesh", "education"},
	{"atsem", "education"},
	{"creche", "education"},
	{"garderie", "education"},
	{"garde d'enfants", "education"},
	{"educateur specialise", "social_services"},
	{"moniteur educateur", "social_services"},
	{"moniteur-educateur", "social_services"},
	{"educateur sportif", "education"},
	{"moniteur d'auto-ecole", "education"},
	{"moniteur auto-ecole", "education"},
	{"animateur periscolaire", "education"},
	{"animateur jeunesse", "education"},
	{"animateur socioculturel", "education"},
	{"accueil de loisirs", "education"},
	{"periscolaire", "education"},
	{"extrascolaire", "education"},
	{"orthopedagogue", "education"},

	// Building trades whose title names an office head. IT maintenance is not a trade.
	{"maintenance informatique", "it_support"},
	{"maintenance applicative", "software_engineering"},
	{"technicien de maintenance", "skilled_trades"},
	{"agent de maintenance", "skilled_trades"},
	{"technicien cvc", "skilled_trades"},
	{"technicien vitrage", "skilled_trades"},
	{"technicien qualite", "manufacturing"},
	{"controleur qualite", "manufacturing"},
	{"controle qualite", "manufacturing"},
	{"assurance qualite", "manufacturing"},
	{"technicien de laboratoire", "science"},
	{"conducteur de ligne", "manufacturing"},
	{"conducteur de machine", "manufacturing"},
	{"conducteur de machines", "manufacturing"},
	{"conducteur d'engin", "skilled_trades"},
	{"conducteur d'engins", "skilled_trades"},
	{"operateur de production", "manufacturing"},
	{"operateur de fabrication", "manufacturing"},
	{"operateur de conditionnement", "manufacturing"},
	{"agent de fabrication", "manufacturing"},
	{"agent de conditionnement", "manufacturing"},
	{"operateur logistique", "logistics"},
	{"agent logistique", "logistics"},
	{"agent de quai", "logistics"},
	{"agent de tri", "logistics"},
	{"operateur de saisie", "administration"},
	{"agent de saisie", "administration"},
	{"agent administratif", "administration"},
	{"employe administratif", "administration"},
	{"agent d'accueil", "administration"},
	{"hote d'accueil", "administration"},
	{"charge d'accueil", "administration"},
	{"hote de caisse", "retail"},
	{"agent d'entretien", "personal_services"},
	{"agent de proprete", "personal_services"},
	{"technicien de surface", "personal_services"}, // Belgium's cleaner
	{"agent de nettoyage", "personal_services"},
	{"agent de menage", "personal_services"},
	{"employe de menage", "personal_services"},
	{"entretien menager", "personal_services"},
	{"femme de chambre", "hospitality"},
	{"valet de chambre", "hospitality"},
	{"receptionniste de nuit", "hospitality"},
	{"receptionniste d'hotel", "hospitality"},
	{"employe polyvalent de restauration", "hospitality"},
	{"equipier polyvalent", "hospitality"},
	{"commis de cuisine", "hospitality"},
	{"commis de salle", "hospitality"},
	{"second de cuisine", "hospitality"},
	{"chef de partie", "hospitality"},
	{"responsable de salle", "hospitality"},
	{"agent de service hotelier", "healthcare"}, // the care home's ASH
	{"agent de service", "personal_services"},
	{"agent de services", "personal_services"},
	{"preparateur drive", "logistics"}, // the supermarket's click-and-collect picker
	{"technicien automobile", "skilled_trades"},
	{"diagnostic automobile", "skilled_trades"},
	{"mecanique automobile", "skilled_trades"},
	{"monteur video", "creative"},
	{"production audiovisuel", "creative"},
	{"audiovisuel", "creative"},
	{"conducteur d'operation", "project_management"}, // a building programme's manager
	{"conducteur d'operations", "project_management"},
	{"cadreur", "creative"},
	{"directeur de magasin", "retail"},
	{"responsable de magasin", "retail"},
	{"adjoint de magasin", "retail"},
	{"responsable de boutique", "retail"},
	{"manager de rayon", "retail"},
	{"directeur de restaurant", "hospitality"},
	{"responsable de restaurant", "hospitality"},
	{"manager de restaurant", "hospitality"},
	{"directeur d'hotel", "hospitality"},
	{"directeur de creche", "education"},
	{"responsable de creche", "education"},
	{"responsable ressources humaines", "hr"},
	{"charge de recrutement", "recruiting"},
	{"charge du recrutement", "recruiting"},
	{"consultant en recrutement", "recruiting"},
	{"chasseur de tetes", "recruiting"},
	{"recruteur", "recruiting"},
	{"recrutement", "recruiting"},
	{"charge de communication", "marketing"},
	{"responsable communication", "marketing"},
	{"responsable de la communication", "marketing"},
	{"relations presse", "marketing"},
	{"redacteur web", "marketing"},
	{"chef de publicite", "marketing"},

	// Band 2: the role nouns that name a trade by themselves.

	// Health.
	{"infirmier", "healthcare"},
	{"infirmiers", "healthcare"},
	{"ide", "healthcare"},
	{"idel", "healthcare"},
	{"medecin", "healthcare"},
	{"idec", "healthcare"},
	{"ophtalmologiste", "healthcare"},
	{"radiologue", "healthcare"},
	{"cardiologue", "healthcare"},
	{"pediatre", "healthcare"},
	{"psychiatre", "healthcare"},
	{"anesthesiste", "healthcare"},
	{"gynecologue", "healthcare"},
	{"dermatologue", "healthcare"},
	{"manipulateur d'electroradiologie", "healthcare"},
	{"medecins", "healthcare"},
	{"pharmacien", "healthcare"},
	{"preparateur en pharmacie", "healthcare"},
	{"kinesitherapeute", "healthcare"},
	{"masseur-kinesitherapeute", "healthcare"},
	{"kine", "healthcare"},
	{"orthophoniste", "healthcare"},
	{"ergotherapeute", "healthcare"},
	{"psychomotricien", "healthcare"},
	{"psychologue", "healthcare"},
	{"neuropsychologue", "healthcare"},
	{"sage-femme", "healthcare"},
	{"sage femme", "healthcare"},
	{"chirurgien-dentiste", "healthcare"},
	{"chirurgien dentiste", "healthcare"},
	{"dentiste", "healthcare"},
	{"orthoptiste", "healthcare"},
	{"opticien", "healthcare"},
	{"audioprothesiste", "healthcare"},
	{"manipulateur en electroradiologie", "healthcare"},
	{"manipulateur radio", "healthcare"},
	{"dieteticien", "healthcare"},
	{"ambulancier", "healthcare"},
	{"brancardier", "healthcare"},
	{"osteopathe", "healthcare"},
	{"podologue", "healthcare"},
	{"veterinaire", "healthcare"},
	{"asv", "healthcare"},
	{"puericulteur", "healthcare"},
	// Education.
	{"enseignant", "education"},
	{"enseignants", "education"},
	{"professeur", "education"},
	{"professeurs", "education"},
	{"formateur", "education"},
	{"formateurs", "education"},
	{"educateur", "education"},
	{"precepteur", "education"},
	// Social work.
	{"travailleur social", "social_services"},
	{"mediateur social", "social_services"},
	// Hospitality.
	{"serveur", "hospitality"},
	{"serveurs", "hospitality"},
	{"cuisinier", "hospitality"},
	{"cuisiniers", "hospitality"},
	{"plongeur", "hospitality"},
	{"barman", "hospitality"},
	{"pizzaiolo", "hospitality"},
	{"crepier", "hospitality"},
	{"maitre d'hotel", "hospitality"},
	{"gouvernant", "hospitality"}, // the hotel's head housekeeper
	{"hotelier", "hospitality"},
	// The trades.
	{"electricien", "skilled_trades"},
	{"electriciens", "skilled_trades"},
	{"plombier", "skilled_trades"},
	{"chauffagiste", "skilled_trades"},
	{"frigoriste", "skilled_trades"},
	{"climaticien", "skilled_trades"},
	{"menuisier", "skilled_trades"},
	{"charpentier", "skilled_trades"},
	{"couvreur", "skilled_trades"},
	{"macon", "skilled_trades"},
	{"carreleur", "skilled_trades"},
	{"platrier", "skilled_trades"},
	{"plaquiste", "skilled_trades"},
	{"peintre en batiment", "skilled_trades"},
	{"soudeur", "skilled_trades"},
	{"chaudronnier", "skilled_trades"},
	{"tuyauteur", "skilled_trades"},
	{"serrurier", "skilled_trades"},
	{"metallier", "skilled_trades"},
	{"mecanicien", "skilled_trades"},
	{"mecaniciens", "skilled_trades"},
	{"electromecanicien", "skilled_trades"},
	{"carrossier", "skilled_trades"},
	{"tolier", "skilled_trades"},
	{"vitrier", "skilled_trades"},
	{"cordiste", "skilled_trades"},
	{"ascensoriste", "skilled_trades"},
	{"poseur", "skilled_trades"},
	{"installateur", "skilled_trades"},
	{"monteur", "skilled_trades"},
	{"ouvrier du batiment", "skilled_trades"},
	{"ouvrier btp", "skilled_trades"},
	{"tourneur", "skilled_trades"},
	{"fraiseur", "skilled_trades"},
	{"usineur", "skilled_trades"},
	{"regleur", "manufacturing"},
	{"jardinier", "personal_services"}, // the main table's "groundskeeper"
	{"paysagiste", "personal_services"},
	{"elagueur", "personal_services"},
	// Logistics.
	{"preparateur de commandes", "logistics"},
	{"preparateur de commande", "logistics"},
	{"preparateur commandes", "logistics"},
	{"cariste", "logistics"},
	{"caristes", "logistics"},
	{"magasinier", "logistics"},
	{"magasiniers", "logistics"},
	{"manutentionnaire", "logistics"},
	{"logisticien", "logistics"},
	{"chauffeur", "logistics"},
	{"chauffeurs", "logistics"},
	{"livreur", "logistics"},
	{"livreurs", "logistics"},
	{"camionneur", "logistics"},
	{"conducteur", "logistics"},
	{"conducteurs", "logistics"},
	{"approvisionneur", "operations"},
	{"acheteur", "operations"}, // the main table's "buyer"
	{"acheteurs", "operations"},
	// Retail.
	{"caissier", "retail"},
	{"caissiers", "retail"},
	{"boucher", "retail"},
	{"poissonnier", "retail"},
	{"charcutier", "retail"},
	{"fromager", "retail"},
	{"fleuriste", "retail"},
	{"vendeur", "sales"},
	{"vendeurs", "sales"},
	{"televendeur", "sales"},
	{"negociateur", "sales"},
	{"prospecteur", "sales"},
	// "Commercial" heads its own title ("Commercial terrain", "Commercial B to B",
	// "Directeur commercial"), so it outranks the fields: "Commercial en assurance"
	// sells insurance. A shopping centre is retail.
	{"centre commercial", "retail"},
	{"commercial", "sales"},
	{"technico-commercial", "sales"},
	// Personal services.
	{"coiffeur", "personal_services"},
	{"coiffeurs", "personal_services"},
	{"barbier", "personal_services"},
	{"barbiers", "personal_services"},
	{"estheticien", "personal_services"},
	{"prothesiste ongulaire", "personal_services"},
	{"toiletteur", "personal_services"},
	{"nettoyeur", "personal_services"},
	// Finance and law.
	{"comptable", "finance"},
	{"comptables", "finance"},
	{"aide-comptable", "finance"},
	{"expert-comptable", "finance"},
	{"expert comptable", "finance"},
	{"controleur de gestion", "finance"},
	{"directeur financier", "finance"},
	{"responsable financier", "finance"},
	{"administratif et financier", "finance"},
	{"tresorier", "finance"},
	{"actuaire", "finance"},
	{"juriste", "legal"},
	{"juristes", "legal"},
	{"avocat", "legal"},
	{"avocats", "legal"},
	{"clerc de notaire", "legal"},
	{"notaire", "legal"},
	{"chimiste", "science"},
	{"laborantin", "science"},

	// Band 3: the fields that give a generic head its category.
	{"informatique", "it_support"},
	{"ressources humaines", "hr"},
	{"rh", "hr"},
	{"rrh", "hr"},
	{"drh", "hr"},
	{"sirh", "hr"},
	{"paie", "finance"}, // the Spanish "nóminas"
	{"comptabilite", "finance"},
	{"controle de gestion", "finance"},
	{"tresorerie", "finance"},
	{"recouvrement", "finance"},
	{"facturation", "finance"},
	{"fiscalite", "finance"},
	{"fiscaliste", "finance"},
	{"audit financier", "finance"},
	{"juridique", "legal"},
	{"droit social", "legal"},
	{"droit des affaires", "legal"},
	{"droit public", "legal"},
	{"droit du travail", "legal"},
	{"droit des societes", "legal"},
	{"droit immobilier", "legal"},
	{"droit fiscal", "legal"},
	{"communication", "marketing"},
	{"evenementiel", "marketing"},
	{"marketing", "marketing"},
	{"graphiste", "design"},
	{"infographiste", "design"},
	{"achat", "operations"},
	{"achats", "operations"},
	{"approvisionnement", "operations"},
	{"vente", "sales"},
	{"ventes", "sales"},
	{"commerciaux", "sales"},
	{"maintenance", "skilled_trades"},
	{"batiment", "skilled_trades"},
	{"btp", "skilled_trades"},
	{"cvc", "skilled_trades"},
	{"plomberie", "skilled_trades"},
	{"electricite", "skilled_trades"},
	{"courant fort", "skilled_trades"}, // the electrician's "CFO" (frenchCFO)
	{"courants forts", "skilled_trades"},
	{"courant faible", "skilled_trades"},
	{"courants faibles", "skilled_trades"},
	{"production", "manufacturing"},
	{"fabrication", "manufacturing"},
	{"usine", "manufacturing"},
	{"conditionnement", "manufacturing"},
	{"logistique", "logistics"},
	{"entrepot", "logistics"},
	{"restauration", "hospitality"},
	{"cuisine", "hospitality"},
	{"hotellerie", "hospitality"},
	{"rayon", "retail"},
	// The supermarket's departments, which head Québec's grocery-clerk titles
	// ("Épicerie Commis temps partiel", "Fruits et légumes Commis").
	{"epicerie", "retail"},
	{"fruits et legumes", "retail"},
	{"boucherie", "retail"},
	{"charcuterie", "retail"},
	{"poissonnerie", "retail"},
	{"produits laitiers", "retail"},
	{"surgeles", "retail"},
	{"expedition", "logistics"},
	{"proprete", "personal_services"},
	{"nettoyage", "personal_services"},
	{"menage", "personal_services"},
	{"accueil", "administration"},
	{"soins", "healthcare"},

	// Band 4: the generic heads that name a desk on their own.
	{"secretaire", "administration"},
	{"secretaires", "administration"},
	{"standardiste", "administration"},
	{"receptionniste", "administration"}, // the main table's "receptionist"
	{"administratif", "administration"},
	{"assistant administratif", "administration"},
	{"assistant de direction", "administration"},
}

// frenchWorkplaceTable lists the WORKPLACES a title names — the bank, the insurer, the
// hotel, the shop, the haulier — read after frenchCategoryTable and only on a title that
// names no technical work (frenchTechTitle). A workplace says what a generic head does
// there ("Conseiller banque", "Agent hôtel"), but on a technical title it is the client's
// sector: "Tech Lead Java - Transport, secteur public", "Data Engineer - Assurance".
var frenchWorkplaceTable = []aliasEntry{
	{"hotel de ville", categoryNone}, // the town hall
	{"credit", "finance"},
	{"bancaire", "finance"},
	{"banque", "finance"},
	{"assurance", "finance"},
	{"assurances", "finance"},
	{"transport", "logistics"},
	{"restaurant", "hospitality"},
	{"hotel", "hospitality"},
	{"magasin", "retail"},
	{"boutique", "retail"},
	{"supermarche", "retail"},
	{"hypermarche", "retail"},
	{"grande distribution", "retail"},
	{"laboratoire", "science"},
	{"pharmacie", "healthcare"},
	{"ehpad", "healthcare"},
}

// frenchTechTitle is a French title naming technical work, whose workplace word is the
// client's sector rather than the job.
var frenchTechTitle = regexp.MustCompile(`\b(?:developpeur|developpement|logiciel|logiciels|informatique|si|it|data|donnees|cloud|ia|cyber\w*|devops|sap|erp|crm|dba|reseau|reseaux|systeme|systemes|web|digital|numerique|tech|java|python)\b`)

// frenchGenderNotice is the gender notice French postings append or weave in: "(H/F)",
// "H/F", "F/H", "(F/H/X)", "H/F/NB", "(H-F)", "M/F", "(h/f/d)".
var frenchGenderNotice = regexp.MustCompile(`\(?\b[hfm]\s*[/\-|]\s*[hfm](?:\s*[/\-|]\s*(?:x|n|nb|d))?\b\)?`)

// frenchInlineGender is the feminine ending a word carries after a point médian, a dot,
// a slash or inside a parenthesis — "développeur·se", "ingénieur.e", "chargé(e)",
// "technicien/ne", "directeur/trice", "conseiller·ère·s", "fonctionnel(le)". It is cut, so
// the masculine word before it matches. The dot is held to the endings that are never
// a file extension or a version ("Vue.js", ".NET").
var (
	frenchInlineGender = regexp.MustCompile(`(\p{L})(?:[·•]|\.|/\s?|-)(?:e|es|ne|nes|le|les|se|euse|euses|rice|rices|trice|trices|ère|ere|ères|eres|ice|fe|ve|sse)(?:[·•.]s)?\b`)
	frenchParenGender  = regexp.MustCompile(`(\p{L})\((?:e|es|ne|le|se|euse|rice|trice|ère|ere|ice|fe|ve|s|sse)\)s?`)
)

// frenchWordFold maps a feminine or plural title word onto the masculine the table
// carries. Its endings are the regular ones: -euse → -eur ("vendeuse"), -trice → -teur
// ("directrice"), -ienne → -ien ("technicienne"), -ière → -ier ("infirmière"), -eure →
// -eur ("ingénieure"), -ée → -é ("chargée"), -elle → -el ("fonctionnelle"), -ale → -al
// ("commerciale"). The irregular ones are listed. A word only folds when the result is
// still a word, so the fold is safe on the place and company names a title carries.
func frenchWordFold(w string) string {
	if m, ok := frenchIrregularFold[w]; ok {
		return m
	}
	for _, s := range frenchSuffixFold {
		if strings.HasSuffix(w, s[0]) && len(w) > len(s[0])+2 {
			return strings.TrimSuffix(w, s[0]) + s[1]
		}
	}
	return w
}

var frenchSuffixFold = [][2]string{
	{"euse", "eur"},
	{"trice", "teur"},
	{"ienne", "ien"},
	{"iere", "ier"},
	{"eure", "eur"},
	{"ee", "e"},
	{"elle", "el"},
	{"ale", "al"},
}

var frenchIrregularFold = map[string]string{
	"assistante":     "assistant",
	"assistantes":    "assistant",
	"consultante":    "consultant",
	"enseignante":    "enseignant",
	"debutante":      "debutant",
	"alternante":     "alternant",
	"gerante":        "gerant",
	"surveillante":   "surveillant",
	"accompagnante":  "accompagnant",
	"intervenante":   "intervenant",
	"commercante":    "commercant",
	"representante":  "representant",
	"agente":         "agent",
	"experte":        "expert",
	"avocate":        "avocat",
	"apprentie":      "apprenti",
	"bouchere":       "boucher",
	"boulangere":     "boulanger",
	"menagere":       "menager",
	"hotesse":        "hote",
	"maitresse":      "maitre",
	"cheffe":         "chef",
	"cheffes":        "chef",
	"administrative": "administratif",
	"educative":      "educatif",
	"gouvernante":    "gouvernant",
	"preposee":       "prepose",
}

// foldFrenchGender cuts the gender notices and inline feminine endings out of a
// lowercased title, keeping its accents: the main table is asked again of it.
func foldFrenchGender(lower string) string {
	lower = strings.NewReplacer("&#39;", "'", "&amp;", "&", "’", "'").Replace(lower)
	lower = frenchParenGender.ReplaceAllString(lower, "$1")
	lower = frenchInlineGender.ReplaceAllString(lower, "$1")
	lower = frenchGenderNotice.ReplaceAllString(lower, " ")
	return strings.Join(strings.Fields(lower), " ")
}

// foldFrench prepares a lowercased title for the French tables: the gender forms cut
// (foldFrenchGender), the accents dropped, and every word folded onto its masculine
// singular (frenchWordFold).
func foldFrench(lower string) string {
	s := foldSpanish(foldFrenchGender(lower))
	var b strings.Builder
	start := -1
	flush := func(end int) {
		if start >= 0 {
			b.WriteString(frenchWordFold(s[start:end]))
			start = -1
		}
	}
	for i, r := range s {
		if r >= 'a' && r <= 'z' {
			if start < 0 {
				start = i
			}
			continue
		}
		flush(i)
		b.WriteRune(r)
	}
	flush(len(s))
	return b.String()
}

// looksFrench reports whether a title shows it is written in French: a French gender
// notice, an accented letter French uses, an elided article, or a French function
// word or contract term. "Stage" is not one: Dutch and English titles carry it too
// ("Data Stage Consultant", "Launch Engineer, Stage 0"). Many of the table's words are also English ones —
// "commercial", "assistant", "agent", "boutique", "restaurant", "production" — and an
// English title the main table left unresolved must not be read as French.
var looksFrenchRe = regexp.MustCompile(`[éèêàçùâîôûëï]|\b[hf]\s*/\s*[hf]\b|\b(?:d|l|qu)'\p{L}|\b(?:de|des|du|et|en|pour|au|aux|chez|avec|sur)\b|\b(?:cdi|cdd|alternance|alternant|stagiaire|apprenti|interim)\b|[·•]|\((?:e|ne|euse|trice|rice)\)`)

func looksFrench(lower string) bool {
	return looksFrenchRe.MatchString(strings.ReplaceAll(lower, "’", "'")) || frenchOnlyHead.MatchString(foldSpanish(lower))
}

// frenchOnlyHead is a head noun no other language reaching the fallback spells this
// way, so a title written in capitals without its accents ("DEVELOPPEUR PYTHON",
// "INGENIEUR RESEAU") still counts as French.
var frenchOnlyHead = regexp.MustCompile(`\b(?:developpeur|developpeuse|ingenieur|ingenieure|technicien|technicienne|conseiller|conseillere|vendeur|vendeuse|responsable|chargee|gestionnaire|infirmier|infirmiere|preparateur|cariste|magasinier|serveur|serveuse|cuisinier|comptable|secretaire|employe|ouvrier|caissier|caissiere|electricien|mecanicien|commerciale|commerciaux|auxiliaire|aide-soignant|aide-soignante|educateur|educatrice|animateur|animatrice|formateur|formatrice|enseignant|enseignante|directeur|directrice|chauffeur|livreur|coiffeur|coiffeuse|logistique|informatique|alternance|stagiaire|juriste|barbier|avocat|medecin|kinesitherapeute|plombier|soudeur|macon|couvreur|menuisier|operateur|operatrice|agente|assistante|analyste|architecte|administrateur|administratrice|consultante|testeur|receptionniste|negociateur|acheteur|approvisionneur|projeteur|dessinateur|automaticien|logisticien|manutentionnaire|plongeur|veterinaire|opticien|pharmacien|psychologue|orthophoniste|chimiste|laborantin|metallier|chaudronnier|carrossier|chauffagiste|frigoriste)\b`)

// frenchChefOf is the French "chef de/d'/des/du X": in English, and in the main
// table, a chef cooks, but French makes the word a head for any lead — "Chef de projet",
// "Chef d'équipe", "Chef de rayon", "Chef de chantier". Only the kitchen's own (partie,
// cuisine, rang, salle, bar, brigade, cave, pâtisserie, réception) keep the main table's
// reading; every other one is rewritten to "cheffe", which the main table does not
// carry and foldFrench maps back to "chef" for the French table.
var frenchChefOf = regexp.MustCompile(`\bchef((?:/fe|\(fe\)|[·.]fe)?\s+(?:d['’]\s*|de\s+(?:la\s+|l['’]\s*)?|des\s+|du\s+))(\p{L}+)`)

var frenchKitchenField = map[string]bool{
	"cuisine": true, "partie": true, "rang": true, "salle": true, "bar": true,
	"brigade": true, "cave": true, "pâtisserie": true, "patisserie": true,
	"réception": true, "reception": true, "plonge": true, "cuisines": true,
}

// frenchCFO is the French electrician's "CFO" — courants forts, the power wiring beside
// "CFA", courants faibles — which the main table reads as the chief financial officer:
// "Technicien de maintenance CFO/CFA", "Conducteur de travaux CFO". It is rewritten only
// on a French title that also names electrical or building work (frenchElectricalWork),
// so "Directeur financier (CFO)" stays the executive.
var (
	frenchCFO            = regexp.MustCompile(`\bcfo\b`)
	frenchElectricalWork = regexp.MustCompile(`\b(?:cfa|maintenance|multitechnique|[ée]lectri\p{L}*|travaux|chantier|technicien\p{L}*|[ée]tudes?|affaires|courants?|ing[ée]nieur\p{L}*|r[ée]f[ée]rent\p{L}*)\b`)
)

// unchefFrench rewrites a French non-kitchen "chef de X" so the main table's "chef" (a
// cook) does not read it, and a French electrician's "CFO" so it does not read the
// executive (frenchCFO).
func unchefFrench(lower string) string {
	lower = strings.ReplaceAll(lower, "&#39;", "'") // an apostrophe some feeds leave escaped
	if strings.Contains(lower, "cfo") && looksFrench(lower) && frenchElectricalWork.MatchString(lower) {
		lower = frenchCFO.ReplaceAllString(lower, "courant fort")
	}
	if !strings.Contains(lower, "chef") {
		return lower
	}
	return frenchChefOf.ReplaceAllStringFunc(lower, func(m string) string {
		sub := frenchChefOf.FindStringSubmatch(m)
		if frenchKitchenField[sub[2]] {
			return m
		}
		return "cheffe" + sub[1] + sub[2]
	})
}

// matchFrenchCategory resolves the category of a lowercased title the other tables
// left unresolved: the main table once more on the gender-cut title, then the French
// table on the folded one. A blind alias answers categoryNone.
func matchFrenchCategory(lower string) string {
	if !looksFrench(lower) {
		return ""
	}
	c := ""
	if g := foldFrenchGender(lower); g != lower {
		c = matchOrdered(g, categoryTable)
	}
	if c != "" {
		return c
	}
	folded := foldFrench(lower)
	if c = matchOrdered(folded, frenchCategoryTable); c == "" && !IsTech(lower) && !frenchTechTitle.MatchString(folded) {
		c = matchOrdered(folded, frenchWorkplaceTable)
	}
	return c
}

// frenchSeniorityTable lists the French grade aliases, whole-word phrases on the folded
// title, in precedence order.
//
// The trainee grades lead, as in German and Spanish: "Stage - Chef de projet" is an
// intern, and so is an apprentice — French "alternance" is a work-study contract a
// student holds beside their degree, the closest this vocabulary has being the intern.
// "Confirmé" is the experienced rung between junior and senior; "Débutant" and "jeune
// diplômé" the junior. The team's own lead is the "chef d'équipe"; "Chef de projet" is
// a desk, not a grade, as "Project Manager" is none, and "responsable", "manager",
// "directeur" and "expert" state none either. Only the general, executive and financial
// directors are the board.
var frenchSeniorityTable = []aliasEntry{
	{"president directeur general", "c_level"},
	{"pdg", "c_level"},
	{"directeur general", "c_level"},
	{"directeur executif", "c_level"},
	{"directeur financier", "c_level"},
	{"directeur administratif et financier", "c_level"},
	{"stagiaire", "intern"},
	{"stagiaires", "intern"},
	{"stage", "intern"},
	{"alternance", "intern"},
	{"alternant", "intern"},
	{"alternants", "intern"},
	{"apprenti", "intern"},
	{"apprentis", "intern"},
	{"apprentissage", "intern"},
	{"contrat de professionnalisation", "intern"},
	{"contrat pro", "intern"},
	{"pfe", "intern"},
	{"chef d'equipe", "lead"},
	{"chef de groupe", "lead"},
	{"responsable d'equipe", "lead"},
	{"manager d'equipe", "lead"},
	{"confirme", "middle"},
	{"debutant", "junior"},
	{"jeune diplome", "junior"},
}

// assistantOfFrenchLead is the assistant OF a lead or an executive — "Assistant(e) de
// direction générale", "Assistante du Directeur Général", "Adjoint au chef d'équipe" —
// which states no grade of its own. It is cut before the French grade match.
var assistantOfFrenchLead = regexp.MustCompile(`\b(?:assistant|secretaire|adjoint|bras droit)\s+(?:(?:de\s+(?:la\s+)?|du\s+|au\s+|a\s+la\s+|des\s+)?)(?:direction generale|directeur general|pdg|chef d'equipe|chef de groupe|daf|directeur financier)\b`)

// frenchCareersDesk is a desk that RUNS the trainee programmes — "Responsable
// alternance", "Chargé de relations écoles et stages", "Gestionnaire des stages" — whose
// "alternance" or "stage" is the programme it looks after, not the holder's grade.
var frenchCareersDesk = regexp.MustCompile(`\b(?:responsable|gestionnaire|coordinateur|charge|chef de projet)\s+(?:(?:des|de l'|de la|du|de)\s+)?(?:relations? ecoles?(?:\s+et\s+(?:stages?|alternance))?|alternance|stages|pole alternance)\b`)

// englishStage is the English "stage" a French reading must not take for an internship:
// "Early-stage", "Seed Stage", "Stage Manager", "Backstage".
var englishStage = regexp.MustCompile(`\b(?:early|seed|late|growth|multi|any|pre-?seed|series [a-d])[\s-]stage\b|\bstage[\s-](?:manager|hand|door|gate|technician|crew)\b`)

// matchFrenchSeniority resolves the grade of a lowercased title the other tables left
// unresolved.
func matchFrenchSeniority(lower string) string {
	if !looksFrench(lower) {
		return ""
	}
	folded := foldFrench(englishStage.ReplaceAllString(lower, " "))
	folded = frenchCareersDesk.ReplaceAllString(folded, " ")
	folded = assistantOfFrenchLead.ReplaceAllString(folded, " ")
	return matchOrdered(folded, frenchSeniorityTable)
}
