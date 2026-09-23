package telegram

import "testing"

// TestLooksLikeVacancySpanish holds the Spanish cohort, and every string in it is a
// VERBATIM post from production (telegram_posts, channel STEMJobsCR) rather than a
// hand-written imitation. That is deliberate: the Ukrainian markers above were scored
// against live posts for the same reason, and a fixture written to match the regex
// proves only that the regex matches itself.
//
// Measured 2026-09-23: the marker set covered RU/EN/UA only, so 6,340 of this channel's
// 6,534 posts (96.8%) were filtered out and never reached the LLM. They are the single
// largest share of the 15,203 posts the prefilter has rejected to date.
//
// The pass cases rest on the channel's BOT TEMPLATE ("Empresa:" / "Ubicación:", with the
// colon), not on bare Spanish hiring vocabulary, and the reject cases are why: the same
// channel carries human chatter that uses "empresa" as an ordinary noun and even
// "contratando" as an ordinary verb, while advertising nothing. A bare-word marker admits
// those; the labelled field does not.
func TestLooksLikeVacancySpanish(t *testing.T) {
	pass := []struct{ name, text string }{
		{"es template empresa+ubicación", "🧑‍💼 | DBA Oracle Senior\nEmpresa: Exceltec Business Solutions\nUbicación: Costa Rica (Remote)\nTags: #data\nhttps://www.linkedin.com/jobs/view/4430120487"},
		{"es template with categoría", "🧑‍💼 | Ayudante de Técnico en CCTV\nEmpresa: Seguridad Iot\nCategoría: Informática / Internet\nUbicación: San José, San José"},
		{"es template on-site", "🧑‍💼 | Jefe de Diseño y Producción\nEmpresa: Aceros Especiales ACES S.A\nUbicación: Uruca, San Jose, Costa Rica (On-site)"},
		{"es template departamento", "🧑‍💼 | Senior Technical Program Manager - CR\nEmpresa: SentinelOne\nDepartamento: Customer Support & Success\nUbicación: Costa Rica"},
		// The channel is inconsistent about the accent on its other fields, which is part of
		// why the marker hangs off "Empresa:" alone rather than the whole template.
		{"es template with an unaccented sibling field", "🧑‍💼 | NPI Engineer I (with Visa)\nEmpresa: Cirtec Medical\nUbicacion: Alajuela, Costa Rica (On-site)"},
	}
	for _, tc := range pass {
		t.Run("pass/"+tc.name, func(t *testing.T) {
			if !LooksLikeVacancy(tc.text) {
				t.Errorf("filtered out a real Spanish vacancy: %q", tc.text)
			}
		})
	}

	// Verbatim human chatter from the SAME channel. Each one holds a Spanish word a
	// naive marker list would reach for, and none of them advertises a job.
	reject := []struct{ name, text string }{
		{"es chatter says empresa", "Creo que les dio vergüenza y borraron todo. No voy a bloquear a esa empresa en el bot, porque quiero ver si vuelven a hacer eso, para seguir reportando"},
		{"es chatter says contratando", "Gente mucho ojo! Me contaron que esta empresa, \"IT Honesto Costa Rica\" está disque contratando \"por servicios profesionales\", le venden la idea a la gente de que tienen varios puestos en TI"},
		{"es chatter says ofertas", "Bloqueé Universal de Tornillos en el bot por spammers, así que si quieren ver las ofertas de esa gente van a tener que ir directo al Linkedin de ellos."},
		{"es chatter asking for help", "🚨ATENCIÓN🚨 Quería pedirles ayuda para mejorar el proyecto: si ven alguna oferta de las bolsas de empleo que el bot monitorea que no haya"},
	}
	for _, tc := range reject {
		t.Run("reject/"+tc.name, func(t *testing.T) {
			if LooksLikeVacancy(tc.text) {
				t.Errorf("let through Spanish chatter that advertises nothing: %q", tc.text)
			}
		})
	}
}
