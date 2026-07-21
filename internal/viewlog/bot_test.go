package viewlog

import "testing"

func TestIsBot(t *testing.T) {
	bots := []string{
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
		"Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)",
		"facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_uatext.php)",
		"Twitterbot/1.0",
		"Slackbot-LinkExpanding 1.0 (+https://api.slack.com/robots)",
	}
	for _, ua := range bots {
		if !isBot(ua) {
			t.Errorf("isBot(%q) = false, want true", ua)
		}
	}

	humans := []string{
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36",
		"curl/8.4.0",
		"",
	}
	for _, ua := range humans {
		if isBot(ua) {
			t.Errorf("isBot(%q) = true, want false", ua)
		}
	}
}
