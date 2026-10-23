package telegram

import (
	"net/url"
	"strings"
	"unicode"
)

// applyURL picks the link a post's single vacancy is applied through, or "" when the post
// names none — in which case the job keeps the post itself as its URL.
//
// A post's links are mostly NOT the vacancy: hashtags, the channel's own Telegram links, and
// footer/ad links a channel stamps on every post (wantapply.com, a WhatsApp channel, a
// "post a vacancy" form). So a link qualifies only when its TEXT says it is the way to apply
// — measured 2026-10-23 over the open single-job Telegram postings, "Apply Here" (LinkedIn),
// "Откликнуться", "ссылке", "тут" carry the per-vacancy link, while a lone link with any
// other text is as often a company homepage or an advert as the vacancy, so it is not
// guessed at. A footer link's text is usually the bare URL itself, which is refused before
// the text is read: wantapply.com would otherwise say "apply" by its own name.
//
// Only a single-vacancy post qualifies: a post naming several roles gives no reliable way to
// tell which link belongs to which, and a wrong pairing sends an applicant to another job.
func applyURL(links []Link, jobs int) string {
	if jobs != 1 {
		return ""
	}
	for _, l := range links {
		if isApplyLink(l) {
			return l.URL
		}
	}
	return ""
}

func isApplyLink(l Link) bool {
	text := strings.TrimSpace(l.Text)
	if text == "" || strings.HasPrefix(text, "#") || isBareURLText(text) {
		return false
	}
	u, err := url.Parse(l.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	if isTelegramHost(u.Hostname()) || isHashtagSearch(u, l.URL) {
		return false
	}
	return saysApply(text)
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
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r)
	})
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
