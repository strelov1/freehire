// Package engagement reads the legal terms a job posting states it engages its worker
// on — vocab.EngagementValues, the `engagement` search facet. Every market has its own
// words for the same question, "employee or own business, and on what tax footing":
// the UK's IR35 status, the US W-2 / 1099 / corp-to-corp split, Poland's B2B against
// the umowa o pracę, Brazil's CLT against PJ, Russia's трудовой договор against ГПХ,
// самозанятость and ИП, and the freelance statuses of Germany, the Netherlands, France,
// Spain and Argentina. Each value is the market's own term, labelled the way that
// market writes it, because a translation into one global ladder would lose exactly
// the distinction a candidate filters on (Inside IR35 and W-2 are both "employed", and
// they are not the same deal).
//
// Like the other dictionaries it is dict-only: a value is emitted only when the text
// states it, never inferred from the rate, the client, the source or the country. An
// empty result means "the posting does not say".
//
// Every rule was measured, not reasoned about. On 2026-11-03 a corpus of 10,496 postings
// was pulled from the live search index — up to 400 per term, each with its own query
// and country — and the wording around every term read by hand; the IR35 rules come
// from the 1,354 postings the search returns for "IR35" the same day. Most terms are
// ambiguous without the words around them, so a rule asks for its ANCHOR — "rodzaj
// umowy:", "contratação", "оформление по", "statut" — and never for the bare term:
//
//   - "B2B" is mostly "B2B sales"; only "B2B:" before a salary, "umowa B2B", "na B2B"
//     and the like name a contract.
//   - "PJ" is a bank's corporate-client segment ("gerente de negócios PJ", "segmento
//     PJ") about as often as it is a contract.
//   - "ИП" is an employer's name ("ИП Комаров") or an accountant's client ("бухгалтер по
//     сопровождению ИП") far more often than an offer; "ФОП" in Russian postings is a
//     school curriculum, so the Ukrainian ФОП is not here at all.
//   - "C2C" is also Mercari's "C2C marketplace", and a recruiter's skills ("experience
//     in C2C/C2H/FTE recruitment").
//   - "autónomo" is also "autonomous", "monotributo" and "1099" also an accountant's
//     duty, "W2" also a German professor's pay grade.
//
// Two guards are shared. A NEGATION right before a term, or governing a list it is in,
// rejects it ("no C2C", "no 1099 or C2C", "not accepting C2C", "this is not an Inside
// IR35 role"); a US posting states what it refuses far more often than other markets
// do. A DUTY rejects a term the work is about rather than the deal ("prepare W-2s",
// "sporządzanie umów", "ведение договоров ГПХ").
//
// Dropped after measurement: Australia's ABN (27 postings, mostly ABN Group and ABN
// AMRO), Ukraine's ФОП and гіг-контракт (19 and 4 Ukrainian postings, swamped by the
// Russian curriculum), and the Czech and Romanian freelance statuses (živnostenský list,
// PFA: none and six).
package engagement

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/strelov1/freehire/internal/platform/htmltext"
)

// rule is one value's matcher. A pattern asserts the value unless a guard rejects the
// match: the shared negation guards, the rule's own duty or noise words before or after
// it, or — for a term that means something else as often as it means a contract — the
// absence of any contract word around it.
type rule struct {
	slug, label string
	// market names where the term is used, for the filter's grouping.
	market string
	// patterns run over the title and description; titlePatterns over the title only,
	// for a term a recruiter's title carries as a tag ("Médico - PJ").
	patterns, titlePatterns []*regexp.Regexp
	// rejectBefore and rejectAfter, when set, disqualify a match the text right before
	// or after it continues into something that is not the deal.
	rejectBefore, rejectAfter *regexp.Regexp
	// context, when set, must match somewhere within contextWindow of the match.
	context *regexp.Regexp
}

// The windows are in bytes and sized for Cyrillic, two bytes a letter: 120 bytes is 60
// Latin letters and 60 Cyrillic ones, the room a duty or a refusal takes to say itself.
const (
	beforeWindow  = 120
	afterWindow   = 80
	contextWindow = 120
)

// B and E bound a term the way \b would if Go's \b knew letters beyond ASCII: a Polish
// "pracę", a Russian "ип" or a Portuguese "contratação" is a word too.
const (
	B = `(?:^|[^\pL\pN])`
	E = `(?:$|[^\pL\pN])`
)

func re(s string) *regexp.Regexp { return regexp.MustCompile(`(?i)` + s) }

// ir35 is the name as postings spell it: "IR35", "IR 35", "IR-35". What follows it may
// not be a digit, which keeps a part number like "IR350" out, but may be a letter: a
// feed that dropped the markup between two lines glues the next word on ("Outside
// IR35We are looking", "Inside IR35November start") — three postings in the sample.
const ir35 = `ir\s?-?35(?:\b|[^0-9])`

// ir35Field is the name used as a field label, with the room a label leaves before its
// value: "IR35: Inside", "IR35 Status: Inside", "IR35 status is outside".
const ir35Field = `\b` + ir35 + `(?:\s+(?:status|determination|classification))?\s*(?:[:\-–—]\s*)?(?:is\s+)?(?:deemed\s+)?`

// ir35Scope is what may sit between a status word and the name: "inside the scope of
// IR35", "outside of IR35".
const ir35Scope = `\s*(?:of\s+)?(?:the\s+)?(?:scope\s+of\s+)?`

// either joins two terms offered side by side: "Inside or Outside IR35", "CLT ou PJ",
// "ГПХ/самозанятость", "B2B lub UoP".
const either = `\s*(?:or|and|/|&|,|ou|или|lub|albo|bzw\.?|oder|o|y)\s*`

var (
	// usTaxDuty is payroll and accounting work on the forms the US terms are named after.
	usTaxDutyBefore = `(?:prepar|process|issu|fil(?:e|ing)\b|reconcil|distribut|year[- ]end|payroll tax|tax form)[^.;:]{0,30}`
	usTaxDutyAfter  = `^\s*(?:(?:and|/|&)\s*(?:w-?2|1099)s?\s+)?(?:forms?|filings?|reporting|reconciliation|processing|preparation|returns?|wages|statements?)\b`
	// recruitingDuty is a recruiter's or a staffing firm's own vocabulary: "experience in
	// IT recruitment (C2C/C2H/FTE)", "through contract staffing and contract-to-hire".
	recruitingDuty = re(`(?:recruit|staffing|sourcing|requisitions?|hiring solutions|talent acquisition)[^.;:]{0,40}$`)
	// plDuty is HR and payroll work on Polish contracts: drawing them up, settling them.
	plDuty = re(`(?:sporządz|sporzadz|przygotow|rozlicz|oblicz|ewidencj|prowadzeni|obsług|naliczan|weryfik|kontrol|znajomoś|wiedz)\pL*[^.;:]{0,40}$|umów\pL*[^.;:]{0,60}$|\bnp\.?\s*(?:\pL+[\s,]+){0,3}$`)
	// ruDuty is HR and accounting work on Russian contracts: knowing the code, keeping
	// the records, drawing the contracts up.
	ruDuty = `(?:знани|ведени|составлени|подготовк|контрол|учёт|учет|сопровождени|бухгалтер|заключени|заключен|расч[её]т|начислени|оформлени[ея]\s+(?:трудовых|договоров|документ))\pL*[^.;:]{0,40}$` +
		// A bank's or a product's clients: "заемщиков (ЮЛ, ИП, самозанятых)".
		`|(?:юл|юридическ\pL*\s+лиц\pL*|заемщик\pL*|заёмщик\pL*|клиент\pL*|исполнител\pL*|подрядчик\pL*)[^.;:]{0,25}$`
	// frRefusal is a French posting refusing the freelance statuses: "poste non ouvert aux
	// freelances, au portage salarial", "exclusivement en CDI (hors prestation, freelance
	// et portage salarial)".
	frRefusal = re(`(?:non ouvert|pas ouvert|hors|sauf|exclusivement en cdi)[^.;:]{0,50}$`)
	// ruClientsAfter is the same clients named after the term: "(ИП, самозанятый или
	// обычное физическое лицо)" lists who a product serves.
	ruClientsAfter = re(`^\pL*\s*(?:или|,)\s*(?:обычн\pL*\s+)?физ\pL*\.?\s*лиц`)
	// esDuty is an accountant's work on Argentine and Spanish tax statuses.
	esDuty = re(`(?:liquidaci|liquidar|gesti[óo]n|asesor|presentaci|declaraci|impuest|contab|ganancias|bienes personales|\biva\b)\pL*[^.;:]{0,40}$`)
)

var rules = []rule{
	// ---- United Kingdom ----
	// IR35, from its own 1,354-posting sample: 886 inside, 435 outside, one both.
	{slug: "ir35_outside", label: "Outside IR35", market: "United Kingdom", patterns: []*regexp.Regexp{
		// "outsdie" is a misspelling read in the sample; "out of scope" is HMRC's own
		// wording for the same determination.
		re(`\b(?:outside|outsdie|out[- ]of[- ]scope)` + ir35Scope + ir35),
		re(ir35Field + `(?:outside|out[- ]of[- ]scope)\b`),
		re(`#outsideir35\b`),
		// "Outside or Inside IR35": the name follows the second status only.
		re(`\boutside` + either + `inside` + ir35Scope + ir35),
		// "IR35: Inside or Outside": the field's second value.
		re(ir35Field + `(?:inside|in[- ]scope)` + either + `(?:outside|out[- ]of[- ]scope)\b`),
	}},
	{slug: "ir35_inside", label: "Inside IR35", market: "United Kingdom", patterns: []*regexp.Regexp{
		// "Umbrellainside IR35" is a template that glued the two words; it is the
		// umbrella company the inside status implies, so the glued form still counts.
		re(`(?:\bumbrella|\b)(?:inside|within|in[- ]scope)` + ir35Scope + ir35),
		re(ir35Field + `(?:inside|in[- ]scope)\b`),
		re(`#insideir35\b`),
		re(`\binside` + either + `outside` + ir35Scope + ir35),
		re(ir35Field + `(?:outside|out[- ]of[- ]scope)` + either + `(?:inside|in[- ]scope)\b`),
	}},

	// ---- United States ----
	// W-2: almost always the posting's own label ("W2 only", "W-2 hourly position",
	// "$45/hr W2"). The duty guards take payroll's forms out; "W2/W3" is a German
	// professorship's pay grade.
	{slug: "w2", label: "W-2", market: "United States",
		patterns:     []*regexp.Regexp{re(B + `w-?2` + E)},
		rejectBefore: re(`(?:` + usTaxDutyBefore + `|besoldung[^.;:]{0,20})$`),
		rejectAfter:  re(usTaxDutyAfter + `|^\s*[/-]?\s*w-?3\b|^\s*-?professur`),
	},
	// 1099: the number is also a price ("$1,099") and an address, so it needs a contract
	// word near it ("1099 contractor", "independent contractor (1099)").
	{slug: "1099", label: "1099", market: "United States",
		patterns:     []*regexp.Regexp{re(B + `1099` + E)},
		rejectBefore: re(`(?:[$€£]\s*|\d[,.]?|form\s+|` + usTaxDutyBefore + `)$`),
		rejectAfter:  re(usTaxDutyAfter + `|^[,.]\d`),
		context:      re(`contract|independent|w-?2|basis|position|role|employment|opportunit|status|engagement|freelance|per diem|self[- ]employed|own boss|\bpay\b|/hr|hourly`),
	},
	// Corp-to-corp: the marketplace sense (Mercari) and the recruiter's skill are
	// rejected, and most of the rest refuse it ("no C2C").
	{slug: "c2c", label: "Corp-to-Corp (C2C)", market: "United States",
		patterns:     []*regexp.Regexp{re(B + `(?:c2c|corp[- ]to[- ]corp|corp2corp)` + E)},
		rejectBefore: recruitingDuty,
		rejectAfter:  re(`^\s*(?:market|platform|app\b|e-?commerce|business|company|commerce|segment|transaction|sales|payments?|resale|model|startup)`),
		context:      re(`contract|w-?2|1099|c2h|\brate|basis|candidate|visa|h-?1b|\bonly\b|accept|allow|position|role|engagement|employment|duration|months|hourly|/hr|vendor|third[- ]part|\bok\b|restriction|tax term`),
	},
	{slug: "contract_to_hire", label: "Contract-to-hire", market: "International",
		patterns: []*regexp.Regexp{
			re(B + `(?:contract[- ]to[- ](?:hire|perm(?:anent)?)|temp[- ]to[- ](?:hire|perm(?:anent)?)|c2h|right[- ]to[- ]hire)` + E),
		},
		rejectBefore: recruitingDuty,
	},

	// ---- Poland and Central Europe ----
	// B2B: only its contract anchors; "B2B sales" is most of what the bare word means.
	{slug: "b2b", label: "B2B contract", market: "Poland & Central Europe",
		patterns: []*regexp.Regexp{
			re(B + `b2b\s*(?:contract|agreement|basis|cooperation|engagement|umow|kontrakt|współprac|-?\s*tätigkeit|-?\s*vertrag)`),
			re(`(?:umow[aęy]|kontrakt(?:u|em)?|contract|współprac[aęy]|cooperation|engagement|forma\s+(?:zatrudnienia|współpracy)\s*:?|rodzaj\s+umowy\s*:?|typ\s+umowy\s*:?|type\s+of\s+(?:contract|employment|agreement)\s*:?|contract\s+type\s*:?|employment\s+type\s*:?|zatrudnienie|na|on|via|through|form\s+of\s+(?:cooperation|employment)\s*:?)\s+(?:a\s+)?b2b` + E),
			// The salary label: "B2B: 20000 – 24000 PLN / mies. netto" (justjoin.it
			// writes every offer's contract types this way), "B2B (netto)".
			re(B + `b2b\s*(?:[:–-]\s*(?:od\s+|from\s+|up to\s+|do\s+)?\d|\(\s*(?:netto|net|\+\s*vat))`),
			re(B + `b2b` + either + `(?:uop|umow[aęy]\s+o\s+prac|employment contract|permanent|umow[aęy]\s+zleceni)`),
			re(`(?:uop|umow[aęy]\s+o\s+prac[ęe]|employment contract|permanent contract)` + either + `b2b` + E),
			re(`(?:netto|net|pln|eur|usd|zł|\d)\s*(?:\+\s*vat\s*)?(?:/\s*\pL+\.?\s*)?\(\s*b2b\s*\)`),
		},
		// A Polish IT title tags the contract in brackets: "Senior Software Engineer (B2B)".
		titlePatterns: []*regexp.Regexp{re(`\(\s*b2b\s*\)`)},
		rejectAfter:   re(`^\s*(?:sales|saas|market|marketing|segment|client|customer|compan|platform|e-?commerce|business|products?|solutions?|software|space|sector|environment|deals?|accounts?|partners?|leads?|services?|team|brand|payments?|ecommerce|sprzeda|klient|rynk)`),
	},
	// The three Polish contracts: CBOP's "Rodzaj umowy:" field states one on every
	// posting, rocketjobs and justjoin.it in the text.
	{slug: "uop", label: "Umowa o pracę", market: "Poland",
		patterns:     []*regexp.Regexp{re(B + `umow[aęy]\s+o\s+prac[ęe]` + E), re(B + `uop` + E)},
		rejectBefore: plDuty,
	},
	{slug: "umowa_zlecenie", label: "Umowa zlecenie", market: "Poland",
		patterns:     []*regexp.Regexp{re(B + `umow[aęy]\s*(?:-\s*)?zleceni[ea]` + E)},
		rejectBefore: plDuty,
	},
	// A rare contract (23 of 400 sampled), offered mostly beside the mandate one.
	{slug: "umowa_o_dzielo", label: "Umowa o dzieło", market: "Poland",
		patterns:     []*regexp.Regexp{re(B + `umow[aęy]\s+o\s+dzie[łl]o` + E)},
		rejectBefore: plDuty,
	},

	// ---- Brazil ----
	// CLT, the labour code's employment: pandapé, empregos, sólides and catho carry it,
	// as "contratação CLT", "regime CLT", "CLT (Efetivo)", or a title tag.
	{slug: "clt", label: "CLT", market: "Brazil",
		patterns: []*regexp.Regexp{
			re(`(?:regime|contrata[çc][ãa]o|contrato|vagas?|modelo|modalidade|tipo\s+de\s+contrato|v[íi]nculo|efetivo|registro|carteira assinada)\s*(?:de\s+|em\s+|:\s*|\(\s*|[-–]\s*)?(?:trabalho\s+)?clt` + E),
			re(B + `clt\s*(?:\(\s*efetivo|efetivo|tempor[áa]rio|\+)`),
			re(B + `clt` + either + `pj` + E),
			re(B + `pj` + either + `clt` + E),
		},
		titlePatterns: []*regexp.Regexp{re(`^\s*clt\s*[-–|:]`), re(`[-–(|]\s*clt\s*\)?\s*$`)},
	},
	// PJ, a contractor's own company. The corporate-client segment is rejected.
	{slug: "pj", label: "PJ", market: "Brazil",
		patterns: []*regexp.Regexp{
			re(`(?:regime|contrata[çc][ãa]o|contrato|vagas?|modelo|modalidade|tipo\s+de\s+contrato|v[íi]nculo)\s*(?:de\s+|como\s+|:\s*|\(\s*|[-–]\s*)?pj` + E),
			re(`prestador(?:\(a\))?\s+de\s+servi[çc]os\s*\(?\s*pj`),
			re(`(?:contrata[çc][ãa]o|contratad[oa]|atua[çc][ãa]o)\s+(?:como\s+)?pessoa\s+jur[íi]dica`),
			re(B + `pj` + either + `clt` + E),
			re(B + `clt` + either + `pj` + E),
		},
		titlePatterns: []*regexp.Regexp{
			re(`[-–(|]\s*pj\s*\)?\s*\.?\s*$`), re(`^\s*pj\s*[-–|:]`), re(`\(\s*pj\s*\)`),
			// "Vendedor PJ", "Recrutador(a) Freelancer PJ", "REMOTO PJ" — a seller's or a
			// recruiter's own company; "Gerente de Negócios PJ" serves corporate clients.
			re(`(?:vendedor|representante|consultor|recrutador|remoto|home office|freelancer)\pL*(?:\(a\))?\s+pj` + E),
		},
		rejectBefore: re(`(?:segmento|carteira|clientes?|associados|empresas|contas?|cr[ée]dito|neg[óo]cios|relacionamento|atendimento|varejo)\s*$`),
	},

	// ---- Russia and the CIS ----
	// The labour code: trudvsem states "оформление по ТК РФ" on most of what it
	// carries. HR's knowledge of the code is the duty.
	{slug: "tk", label: "Трудовой договор (ТК)", market: "Russia & CIS",
		patterns: []*regexp.Regexp{
			re(`(?:оформлени\pL*|трудоустройств\pL*|работа|работаешь|официальн\pL*)\s+(?:[\pL.]+\s+){0,4}?(?:по|согласно|в\s+соответствии\s+с)\s+тк` + E),
			re(B + `(?:по\s+)?трудов(?:ой|ому)\s+договор(?:у)?` + E),
			re(B + `тк\s*(?:рф|рк|рб)?` + either + `(?:гпх|самозанят|ип` + E + `)`),
		},
		rejectBefore: re(ruDuty),
	},
	{slug: "gph", label: "ГПХ", market: "Russia & CIS",
		patterns: []*regexp.Regexp{
			re(`(?:договор[ау]?|по|оформлени\pL*\s+по)\s+гпх` + E),
			re(B + `гпх` + either), re(either + `гпх` + E),
			re(`\(\s*гпх\s*\)`),
			re(B + `гражданско-правов(?:ой|ому)\s+договор` + E),
		},
		rejectBefore: re(`(?:договоров|договоры|договорам|договорами)\s*$|` + ruDuty),
	},
	{slug: "samozanyaty", label: "Самозанятый", market: "Russia & CIS",
		patterns: []*regexp.Regexp{
			re(`(?:оформлени\pL*|работ\pL*|сотрудничеств\pL*|договор\pL*)\s+(?:\pL+\s+){0,2}?(?:как|в\s+качестве|через|по|с)\s+(?:статус\pL*\s+)?самозанят`),
			re(`самозанят\pL*` + either + `(?:ип` + E + `|гпх|тк)`),
			re(`(?:ип|гпх|тк\s*рф)` + either + `самозанят`),
			re(B + `статус\pL*\s+самозанят`),
		},
		rejectBefore: re(ruDuty),
		rejectAfter:  ruClientsAfter,
	},
	{slug: "ip", label: "ИП", market: "Russia & CIS",
		patterns: []*regexp.Regexp{
			re(`(?:оформлени\pL*|работ\pL*|сотрудничеств\pL*|договор\pL*)\s+(?:\pL+\s+){0,2}?(?:как|в\s+качестве|через|по|с)\s+ип` + E),
			re(B + `ип` + either + `(?:самозанят|гпх)`),
			re(`(?:самозанят\pL*|гпх)` + either + `ип` + E),
			re(B + `статус\pL*\s+ип` + E),
		},
		rejectBefore: re(ruDuty),
		rejectAfter:  ruClientsAfter,
	},

	// ---- Germany ----
	// Freelance: mostly lecturers ("Dozent (freiberuflich)"); the bare noun is as often
	// a client group ("Handwerker, Freiberufler und vermögende Privatpersonen").
	{slug: "freiberuflich", label: "Freiberuflich", market: "Germany",
		patterns: []*regexp.Regexp{
			re(`auf\s+freiberuflicher\s+basis`),
			re(B + `(?:als|eine?n?|für)\s+freiberufliche[rn]?(?:\s*/\s*r)?\s+\pL+`),
			re(`\(\s*freiberuflich\s*\)`),
			re(B + `freiberufliche\s+(?:tätigkeit|mitarbeit|zusammenarbeit|basis)`),
		},
		titlePatterns: []*regexp.Regexp{re(`freiberuf`)},
	},
	// Agency employment: the contract-type field and "im Rahmen der Arbeitnehmerüber-
	// lassung" about the position; an agency describing its own portfolio is not one.
	{slug: "arbeitnehmerueberlassung", label: "Arbeitnehmerüberlassung", market: "Germany",
		patterns: []*regexp.Regexp{
			re(`(?:vertragsart|anstellungsart|beschäftigungsart|art\s+der\s+anstellung)\s*:?\s*[^.;]{0,30}arbeitnehmerüberlassung`),
			re(`\(\s*arbeitnehmerüberlassung\s*\)`),
			re(`(?:stelle|position|tätigkeit|anstellung|einsatz|beschäftigung)\s+(?:ist|erfolgt|im|in)\s+(?:\pL+\s+){0,3}?arbeitnehmerüberlassung`),
			re(`im\s+rahmen\s+(?:der|einer)\s+arbeitnehmerüberlassung\s+(?:mit\s+option|bei|beim|für|zu|an)`),
			re(B + `in\s+arbeitnehmerüberlassung`),
		},
	},

	// ---- Netherlands ----
	{slug: "zzp", label: "ZZP", market: "Netherlands",
		patterns: []*regexp.Regexp{
			re(B + `(?:als|op)\s+(?:een\s+)?zzp(?:['’]?er|[- ]?basis)?` + E),
			re(B + `zzp[- ]?basis` + E),
			re(B + `zzp\s+(?:or|of)\s+(?:own company|eigen|bv|loondienst|detachering)`),
			re(`(?:freelance|freelancer)\s*\(\s*zzp`),
			re(`\(\s*zzp\s*\)`),
		},
		titlePatterns: []*regexp.Regexp{re(B + `zzp` + E)},
	},

	// ---- France ----
	{slug: "auto_entrepreneur", label: "Auto-entrepreneur", market: "France",
		patterns: []*regexp.Regexp{
			re(`statut\s+(?:d['’]\s*)?(?:auto|micro)[- ]?entrepreneur`),
			re(`(?:en\s+tant\s+qu['’]\s*|vous\s+êtes\s+|êtes[- ]vous\s+|etes\s+)(?:auto|micro)[- ]?entrepreneur`),
			re(`\(\s*(?:auto|micro)[- ]?entrepreneur\s*\)`),
		},
		titlePatterns: []*regexp.Regexp{re(`(?:auto|micro)[- ]?entrepreneur(?:e?s)?` + E)},
		rejectBefore:  frRefusal,
	},
	{slug: "portage_salarial", label: "Portage salarial", market: "France",
		patterns:     []*regexp.Regexp{re(`(?:en|ou|et|via|par|au)\s+portage\s+salarial`)},
		rejectBefore: frRefusal,
	},

	// ---- Spain and Argentina ----
	// Autónomo: a self-employed driver, salesperson or professional; "trabajo autónomo"
	// is working on one's own, and is not here.
	{slug: "autonomo", label: "Autónomo", market: "Spain",
		patterns: []*regexp.Regexp{
			re(`(?:como|en\s+r[ée]gimen\s+de|r[ée]gimen\s+de|alta\s+(?:como|de)|dado\s+de\s+alta\s+como|darse\s+de\s+alta\s+como|contrato\s+de|cuota\s+de)\s+aut[óo]nom[oa]s?` + E),
			re(B + `(?:trabajador|comercial|conductor|colaborador|profesional|repartidor|agente)(?:e?s)?(?:\s*/\s*a)?\s+aut[óo]nom[oa]s?` + E),
		},
		rejectBefore: esDuty,
	},
	// Monotributo: nearly every posting naming it asks the candidate to have it.
	{slug: "monotributo", label: "Monotributo", market: "Argentina",
		patterns:     []*regexp.Regexp{re(B + `monotribut(?:o|ista)` + E)},
		rejectBefore: esDuty,
	},
}

var (
	// negatedBefore rejects a term a negation governs right before it: "no C2C", "not
	// accepting C2C", "this is not an Inside IR35 role", "not to be inside IR35".
	negatedBefore = re(`(?:^|[^\pL])(?:not|no|non|never|without|cannot|can't|isn'?t|не|без|nie|bez|sem|não|nao|sin|ohne|keine?n?|geen|sans)\s+(?:to\s+)?(?:be\s+)?(?:an?\s+)?(?:[a-z]+\s+){0,2}$`)
	// negatedList rejects one governing the list the term closes: "no 1099 or C2C", "no
	// visa support or C2C".
	negatedList = re(`(?:^|[^\pL])(?:no|not|without|non|hors|sauf|nie|bez|sem|não|sin|ohne|kein\pL*|geen|не|без)\s+[^.;:!?|\n]{0,40}(?:\s(?:or|and|nor|et|ou|ni|lub|albo|или|oder|of)\s+(?:[\pL\pN-]+\s*)?|\s*/\s*)$`)
	// negatedAfter rejects a term the text goes on to refuse: "C2C not accepted", "C2C
	// and C2H will not be considered".
	// negatedListEN is the English refusal of a comma list: "cannot accept H-1B, F-1/STEM
	// OPT, 1099, C2C, or C2H". Commas join a list only after an English refusal; a
	// Polish "Bez doświadczenia, umowa o pracę" is a requirement and an offer.
	negatedListEN = re(`(?:^|[^\pL])(?:no|not|cannot|can't|won't|unable to|without)\s+[^.;:!?|\n]{0,40},\s*(?:or\s+)?$`)
	negatedAfter  = re(`^\s*\)?\s*(?:(?:and|or|/|,|&)\s*[\w-]+\s*)?(?:is|are|will|would|can|do|does)?\s*(?:not|n't|cannot)\b`)
)

// Labels maps every value to its display name, for the filter chips (generated into
// the SPA's contracts so the two cannot drift).
func Labels() map[string]string {
	out := make(map[string]string, len(rules))
	for _, r := range rules {
		out[r.slug] = r.label
	}
	return out
}

// Markets maps every value to the market whose term it is, for the filter's grouping.
func Markets() map[string]string {
	out := make(map[string]string, len(rules))
	for _, r := range rules {
		out[r.slug] = r.market
	}
	return out
}

// FromText returns the engagement terms a posting's title and description state, in
// vocab.EngagementValues order, or nil when they state none. Both are read: a
// recruiter's title carries the term about as often as the body does. The description
// is read as visible text, so a tag between a field label and its value cannot hide the
// value.
func FromText(title, description string) []string {
	title = clean(title)
	// A full stop between the two keeps a title's last word and the body's first from
	// reading as one phrase.
	text := title + ". " + clean(htmltext.ToText(description))
	var out []string
	for _, r := range rules {
		if r.asserted(text, title) {
			out = append(out, r.slug)
		}
	}
	return out
}

// clean folds whitespace and drops the asterisks the text renderer marks bold with,
// which would sit between a field label and its value ("*IR35:* Inside").
func clean(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "*", " ")), " ")
}

func (r rule) asserted(text, title string) bool {
	for _, p := range r.titlePatterns {
		if p.MatchString(title) {
			return true
		}
	}
	for _, p := range r.patterns {
		// A refusal carries over to the term's alias right after it: "NOT open to
		// corp2corp (C2C)" refuses the "(C2C)" as well.
		refusedUntil := -1
		for _, loc := range p.FindAllStringIndex(text, -1) {
			if loc[0] <= refusedUntil {
				continue
			}
			switch r.accepts(text, loc[0], loc[1]) {
			case accepted:
				return true
			case refused:
				refusedUntil = loc[1] + aliasWindow
			}
		}
	}
	return false
}

// verdict is what the guards make of one match: a refusal is told apart from any other
// rejection because it carries over to an alias right after the match.
type verdict int

const (
	accepted verdict = iota
	refused
	rejected
)

// aliasWindow is how far past a refused match its refusal carries.
const aliasWindow = 12

// accepts applies the guards to one match at text[start:end].
func (r rule) accepts(text string, start, end int) verdict {
	// B and E consume the character beside a term; the guards read from the term itself.
	start, end = trimBoundary(text, start, end)
	before := text[runeStart(text, start-beforeWindow):start]
	after := text[end:runeEnd(text, end+afterWindow)]
	if negatedBefore.MatchString(before) || negatedList.MatchString(before) || negatedListEN.MatchString(before) || negatedAfter.MatchString(after) {
		return refused
	}
	if r.rejectBefore != nil && r.rejectBefore.MatchString(before) {
		return rejected
	}
	if r.rejectAfter != nil && r.rejectAfter.MatchString(after) {
		return rejected
	}
	if r.context != nil {
		around := text[runeStart(text, start-contextWindow):runeEnd(text, end+contextWindow)]
		if !r.context.MatchString(around) {
			return rejected
		}
	}
	return accepted
}

// trimBoundary moves a match's edges past the non-letters B and E consumed.
func trimBoundary(s string, start, end int) (int, int) {
	for start < end {
		r, n := utf8.DecodeRuneInString(s[start:])
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '(' || r == '#' {
			break
		}
		start += n
	}
	for end > start {
		r, n := utf8.DecodeLastRuneInString(s[:end])
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ')' {
			break
		}
		end -= n
	}
	return start, end
}

// runeStart and runeEnd clamp a window edge to the text and move it off the middle of a
// multi-byte letter, which a byte offset into Cyrillic or Polish text lands in often.
func runeStart(s string, i int) int {
	if i <= 0 {
		return 0
	}
	for i < len(s) && !utf8.RuneStart(s[i]) {
		i++
	}
	return i
}

func runeEnd(s string, i int) int {
	if i >= len(s) {
		return len(s)
	}
	for i > 0 && !utf8.RuneStart(s[i]) {
		i--
	}
	return i
}
