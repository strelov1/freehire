package telegram

import (
	"net/url"
	"slices"
	"strings"
	"unicode"
)

// applyURL picks the link a post's single vacancy is applied through, or "" when the post
// names none — in which case the job keeps the post itself as its URL.
//
// A post's links are mostly NOT the vacancy: hashtags, the channel's own Telegram links, and
// footer/ad links a channel stamps on every post (wantapply.com, a WhatsApp channel, a
// "post a vacancy" form). So a link qualifies only when the post's own layout says it is the
// way to apply, read in this order:
//
//  1. The link's own text. Measured 2026-10-23 over the open single-job Telegram postings,
//     "Apply Here" (LinkedIn), "Откликнуться", "ссылке", "тут" carry the per-vacancy link,
//     while a lone link with any other text is as often a company homepage or an advert as
//     the vacancy, so it is not guessed at. A footer link's text is usually the bare URL
//     itself, which this reading refuses: wantapply.com would otherwise say "apply" by its
//     own name.
//  2. The label in front of it (applyLabelled). Many channels print the vacancy's URL itself,
//     so its text is the bare address and only the words before it say what it is:
//     "Link: https://jobs.ea.com/…", "Apply here: …", or "See full details and apply:" on the
//     line above. A footer's URL sits behind words that are not a label ("Join
//     https://whatsapp.com/… for more", "Follow us on Twitter: …") and stays refused.
//  3. The post's ONLY outbound link, when it is a bare address printed on a line of its own
//     and names a page rather than a site (soleAddress). Measured 2026-10-29 over the open
//     single-job postings, 3,580 posts have that shape — STEMJobsCR alone prints every
//     vacancy as a title, three facts and the URL — and only 113 of those addresses recur
//     in another post of the same channel, mostly the same vacancy posted twice. A footer
//     prints its address beside words, so it does not take this shape; and a post with a
//     second outbound link is ambiguous, so it does not qualify.
//
// The first two readings take the first qualifying link in the post's order.
//
// Only a single-vacancy post qualifies: a post naming several roles gives no reliable way to
// tell which link belongs to which, and a wrong pairing sends an applicant to another job.
func applyURL(text string, links []Link, jobs int) string {
	if jobs != 1 {
		return ""
	}
	var outbound []Link
	for _, l := range links {
		if isOutbound(l) {
			outbound = append(outbound, l)
		}
	}
	for _, l := range outbound {
		if saysApplyItself(l) || applyLabelled(text, l) {
			return l.URL
		}
	}
	if len(outbound) == 1 && soleAddress(text, outbound[0]) {
		return outbound[0].URL
	}
	return ""
}

// isOutbound reports whether a link leaves Telegram for an http(s) page and is not a hashtag:
// the only links that can be a vacancy's own page.
func isOutbound(l Link) bool {
	text := strings.TrimSpace(l.Text)
	if text == "" || strings.HasPrefix(text, "#") {
		return false
	}
	u, err := url.Parse(l.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	return !isTelegramHost(u.Hostname()) && !isHashtagSearch(u, l.URL)
}

// saysApplyItself reports a link whose words, not its address, say it is the way to apply.
func saysApplyItself(l Link) bool {
	text := strings.TrimSpace(l.Text)
	return !isBareURLText(text) && saysApply(text)
}

// applyLabelled reports a link the post introduces as the apply link: the words between the
// start of its line and the link end in an apply label, or — when the link opens its line —
// the nearest non-empty line above ends in one. Only the label's end is read, so "Apply
// here:", "👉 Apply link:" and "See full details and apply:" all qualify while "Join … for
// more opportunities" does not.
func applyLabelled(text string, l Link) bool {
	above, lead, _, ok := linkLine(text, l)
	if !ok {
		return false
	}
	if words := labelWords(lead); len(words) > 0 {
		return endsInApplyLabel(words)
	}
	for _, prev := range slices.Backward(strings.Split(above, "\n")) {
		if words := labelWords(prev); len(words) > 0 {
			return endsInApplyLabel(words)
		}
	}
	return false
}

// soleAddress reports a link that is a bare address standing on a line of its own (nothing
// beside it but emoji and punctuation) and that names a page: a bare domain is the company's
// homepage, not its vacancy.
func soleAddress(text string, l Link) bool {
	if !isBareURLText(strings.TrimSpace(l.Text)) {
		return false
	}
	_, lead, trail, ok := linkLine(text, l)
	if !ok || len(labelWords(lead+" "+trail)) > 0 {
		return false
	}
	u, err := url.Parse(l.URL)
	return err == nil && (strings.Trim(u.Path, "/") != "" || u.RawQuery != "")
}

// linkLine finds where a link's text first appears in the post and splits the post around
// it: everything above its line, what precedes it on the line, and what follows it there.
func linkLine(text string, l Link) (above, lead, trail string, ok bool) {
	linkText := strings.TrimSpace(l.Text)
	at := strings.Index(text, linkText)
	if at < 0 {
		return "", "", "", false
	}
	start := strings.LastIndexByte(text[:at], '\n') + 1
	trail, _, _ = strings.Cut(text[at+len(linkText):], "\n")
	return text[:max(start-1, 0)], text[start:at], trail, true
}

// labelWords lowercases a label and splits it into words, dropping the colons, arrows and
// emoji that decorate it.
func labelWords(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r)
	})
}

// applyLabelEndings are the phrases a label introducing a vacancy's link ends in, matched on
// whole words. Each was read off the open single-job postings on 2026-10-29 ("Apply here:",
// "Apply via:", "Apply link:", "Click to apply:", "See full details and apply:",
// "Откликнуться:"). A label merely MENTIONING applying elsewhere in the line does not count:
// only its last words do, which is what separates "Apply here:" from "Join our channel to
// apply faster:".
var applyLabelEndings = [][]string{
	{"apply"},
	{"apply", "here"},
	{"apply", "now"},
	{"apply", "via"},
	{"apply", "at"},
	{"apply", "online"},
	{"apply", "link"},
	{"application", "link"},
	{"apply", "directly"},
	{"откликнуться"},
	{"откликнуться", "на", "вакансию"},
}

// applyLabelWhole are labels that say "this is the vacancy's link" only when they are the
// label's ONLY words: "Link:" introduces the posting, while "WhatsApp community link:" ends in
// the same word and introduces the channel's footer.
var applyLabelWhole = [][]string{
	{"link"},
	{"job", "link"},
	{"ссылка"},
	{"ссылка", "на", "вакансию"},
	{"посилання"},
}

func endsInApplyLabel(words []string) bool {
	for _, label := range applyLabelEndings {
		if len(words) >= len(label) && slices.Equal(words[len(words)-len(label):], label) {
			return true
		}
	}
	return slices.ContainsFunc(applyLabelWhole, func(label []string) bool {
		return slices.Equal(words, label)
	})
}

// isBareURLText reports whether a link's text is an address rather than words: a single
// token carrying a dot or a slash ("wantapply.com", "https://www.linkedin.com/jobs/view/44…").
func isBareURLText(text string) bool {
	return !strings.ContainsFunc(text, unicode.IsSpace) && strings.ContainsAny(text, "./")
}

// isHashtagSearch reports a Telegram hashtag link, "?q=%23tag" — or "?q=#tag", where the
// unescaped "#" turns the tag into a fragment and leaves q empty.
func isHashtagSearch(u *url.URL, raw string) bool {
	return strings.HasPrefix(u.Query().Get("q"), "#") || strings.Contains(raw, "?q=#")
}

func isTelegramHost(host string) bool {
	host = strings.TrimPrefix(strings.ToLower(host), "www.")
	return host == "t.me" || host == "telegram.me" || host == "telegram.dog"
}

// applyWords are the link texts that name the way to apply, matched as whole words: "тут"
// must not match "институт", nor "here" match "where".
var applyWords = map[string]bool{
	"apply": true, "here": true, "тут": true, "здесь": true, "посиланням": true,
}

// applyStems match a word by its start, covering the inflections of one verb or noun:
// откликнуться/отклик/откликайтесь, ссылке/ссылка/ссылку.
var applyStems = []string{"отклик", "ссылк"}

// applyPhrases are two-word texts that point at the vacancy's own page.
var applyPhrases = [][2]string{
	{"описание", "вакансии"},
	{"job", "description"},
}

func saysApply(text string) bool {
	words := labelWords(text)
	for i, w := range words {
		if applyWords[w] {
			return true
		}
		for _, stem := range applyStems {
			if strings.HasPrefix(w, stem) {
				return true
			}
		}
		if i+1 < len(words) {
			for _, p := range applyPhrases {
				if w == p[0] && words[i+1] == p[1] {
					return true
				}
			}
		}
	}
	return false
}
