package classify

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Spanish titles are not compounds: the role is a HEAD NOUN followed by the field it
// works in — "Técnico de mantenimiento", "Auxiliar de cocina", "Jefe de obra",
// "Ejecutivo de ventas". Heads like asesor, técnico, ejecutivo, operario, auxiliar,
// gerente, analista and jefe carry no category of their own; the field after "de" does.
//
// So the table is ordered in four bands, and table order is the precedence:
//
//  1. a head and field whose answer differs from the field's default ("Ingeniero de
//     ventas" is pre-sales engineering, "Auxiliar de enfermería" is a nurse);
//  2. the role nouns that name a trade by themselves (camarero, electricista,
//     conductor, cajero) — they outrank the field, so "Mozo de almacén para tienda"
//     moves goods rather than selling them;
//  3. the fields themselves (ventas, mantenimiento, cocina, almacén, limpieza), which
//     give a generic head its category;
//  4. the generic heads that name a desk on their own (administrativo, comercial).
//
// A generic head with no field — técnico, auxiliar, ayudante, operario, asesor,
// gerente, analista, especialista, consultor, coordinador — resolves nothing, as
// "Specialist" and "Coordinator" resolve nothing in the main table.
//
// Every alias is written without accents and in the masculine where the feminine is a
// separate word ("enfermero", "enfermera"): titles are written both with and without
// accents ("TÉCNICO", "Tecnico"), so the title is folded before matching (see
// foldSpanish), and the slash and parenthesised gender forms ("Camarero/a",
// "Asesor(a)", "(h/m)") are cut.
//
// The table is a FALLBACK: it runs only after the main, German, Dutch and Danish
// tables resolved nothing, so no existing answer changes. Before it, the main table is
// asked once more of the gender-folded title, because "Operario/a de producción" spells
// an alias the main table already carries with its gender suffix in the way. Every
// entry must also be safe on the English, Portuguese, Italian and French titles that
// reach the same fallback; the ones that need it are argued beside their entry.

// spanishGenderMarker is the gender notice Spanish postings append, "(h/m)", "(M/F)",
// "(m/f/d)" — genderMarker's set plus "h" (hombre).
var spanishGenderMarker = regexp.MustCompile(`\(\s*[hmwdfxv](?:\s*[/|,·]\s*[hmwdfxv])+\s*\)`)

// spanishGenderSlash is the slashed or hyphenated feminine or plural ending,
// "Camarero/a", "Asesor/a", "Operario/as", "Operarios/a", "Encargado-a". The ending is
// cut, so the masculine head that precedes it matches.
var spanishGenderSlash = regexp.MustCompile(`(\p{L})[/-](?:as|os|es|a|o|ra|ora)\b`)

// foldSpanishGender cuts the gender forms out of a lowercased title.
func foldSpanishGender(lower string) string {
	lower = spanishGenderMarker.ReplaceAllString(lower, " ")
	lower = spanishGenderSlash.ReplaceAllString(lower, "$1")
	return strings.Join(strings.Fields(lower), " ")
}

// foldSpanish prepares a lowercased title for the Spanish tables: the gender forms are
// cut, the accents dropped ("técnico" → "tecnico", "diseñador" → "disenador") and the
// ordinal indicators spelled as letters ("1ª" → "1a").
func foldSpanish(lower string) string {
	lower = foldSpanishGender(lower)
	var b strings.Builder
	for _, r := range norm.NFD.String(lower) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case r == 'ª':
			b.WriteRune('a')
		case r == 'º':
			b.WriteRune('o')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// matchSpanishCategory resolves the category of a lowercased title the other tables
// left unresolved: the main table once more on the gender-folded title, then the
// Spanish table on the folded one. A blind alias answers categoryNone, so the caller
// can tell a veto from a miss before it asks the Portuguese table.
func matchSpanishCategory(lower string) string {
	c := ""
	if g := foldSpanishGender(lower); g != lower {
		c = matchOrdered(g, categoryTable)
	}
	if c == "" {
		c = matchOrdered(foldSpanish(lower), spanishCategoryTable)
	}
	return c
}

// spanishSeniorityTable lists the Spanish grade aliases, whole-word phrases on the
// folded title, in precedence order.
//
// "Jefe" leads, as the German "-leiter" and the Dutch "-leider" do; "responsable",
// "gerente", "supervisor" and "coordinador" state no grade, as "manager", "supervisor"
// and "coordinator" state none in the main table. A plain "director" is left alone for
// the same reason; only the general, executive and financial directors are the board.
var spanishSeniorityTable = []aliasEntry{
	// Argentina's "Gerente General" is the general manager, the executive.
	{"gerente general", "c_level"},
	{"director general", "c_level"},
	{"directora general", "c_level"},
	{"director ejecutivo", "c_level"},
	{"directora ejecutiva", "c_level"},
	{"director financiero", "c_level"},
	{"directora financiera", "c_level"},
	{"consejero delegado", "c_level"},
	{"consejera delegada", "c_level"},
	// The trainee grades outrank the lead, as in German: "Becario Jefe de Proyecto"
	// is an intern.
	{"becario", "intern"},
	{"becaria", "intern"},
	{"becarios", "intern"},
	{"beca", "intern"},
	{"en practicas", "intern"},
	{"practicas", "intern"},
	{"practicante", "intern"},
	{"practicantes", "intern"},
	{"pasante", "intern"},
	{"pasantes", "intern"},
	{"pasantia", "intern"},
	{"aprendiz", "intern"},
	// Portuguese reaches this fallback too (Brazil's pandape, gupy, empregos), and its
	// internship resolves nowhere else. Its middle grade, "pleno", is read by the
	// Portuguese table (portuguese.go) after "sênior", so a range keeps its upper bound.
	{"estagio", "intern"},
	{"estagiario", "intern"},
	{"estagiaria", "intern"},
	{"estagiarios", "intern"},
	{"jefe", "lead"},
	{"jefa", "lead"},
	{"jefes", "lead"},
	{"lider de", "lead"},
	{"lider tecnico", "lead"},
}

// assistantOfSpanishLead is the assistant OF a lead or an executive — "Ayudante de jefe de
// obra", "Adjunto al jefe de tienda", "Subjefe de turno", "Segundo jefe", "Asistente del
// Director General", "Secretaria de dirección general" — which states no grade of its own.
// It is cut before the Spanish grade match, as assistantOfLead is before the German one.
var assistantOfSpanishLead = regexp.MustCompile(`\b(?:(?:ayudante|asistente|adjunto|adjunta|auxiliar|secretari[oa])\s+(?:(?:del?|al|a la)\s+)?(?:jef[ea]s?|director[a]?\s+(?:general|ejecutiv[oa]|financier[oa])|direccion\s+(?:general|financiera)|gerente\s+general)|subjef[ea]s?|segund[oa]\s+jef[ea])\b`)

// openingLider is a title that OPENS with "Líder": "Líder Comercial", "Lider Tester",
// "Líder DevOps" name the lead. Anywhere else the word praises the employer ("Repositor/a
// Consumo Masivo (Empresa Líder)", "compañía líder en su rubro"), so only the opening word
// is read, as the "lider de" the table already carries.
var openingLider = regexp.MustCompile(`^[^\p{L}\p{N}]*lider\b`)

// foldSpanishSeniority prepares a lowercased title for the Spanish grade match.
func foldSpanishSeniority(lower string) string {
	folded := openingLider.ReplaceAllString(foldSpanish(lower), "lider de")
	return assistantOfSpanishLead.ReplaceAllString(folded, " ")
}
