package skilltag

import (
	"slices"
	"testing"
)

// TestParse_MarketingPlatforms covers the lifecycle, email and advertising
// platforms. "Google Ads" is the interesting one: the phrase pass must claim it
// before the word pass can read "google" as some other Google product.
func TestParse_MarketingPlatforms(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{"klaviyo", "Own flows in Klaviyo.", []string{"klaviyo"}},
		{"mailchimp", "Campaigns in Mailchimp.", []string{"mailchimp"}},
		{"customer.io", "Journeys built in Customer.io.", []string{"customer-io"}},
		{"google ads", "Manage Google Ads budgets.", []string{"google-ads"}},
		{"meta ads", "Scale Meta Ads creative.", []string{"meta-ads"}},
		{"facebook ads is the same canonical", "Scale Facebook Ads creative.", []string{"meta-ads"}},
		{"tiktok ads", "Launch TikTok Ads campaigns.", []string{"tiktok-ads"}},
		{"linkedin ads", "Run LinkedIn Ads for ABM.", []string{"linkedin-ads"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.text)
			for _, w := range tc.want {
				if !slices.Contains(got, w) {
					t.Errorf("Parse(%q) = %v, want it to contain %q", tc.text, got, w)
				}
			}
		})
	}
}

// TestParse_GoogleAdsDoesNotLeakAnotherGoogleProduct pins the vendor-collision
// guard: naming one Google product must not emit a different one.
func TestParse_GoogleAdsDoesNotLeakAnotherGoogleProduct(t *testing.T) {
	got := Parse("Manage Google Ads budgets across accounts.")
	for _, unwanted := range []string{"google-analytics", "gcp"} {
		if slices.Contains(got, unwanted) {
			t.Errorf("Parse(...) = %v, want no %q", got, unwanted)
		}
	}
}

// TestParse_MarketingMeasurementAndSocialTooling covers the measurement and social
// stack. Several products in this space are named after ordinary English words —
// Segment, Buffer, Later — and are deliberately absent from the vocabulary: a bare
// "segment" is a customer segment in the very postings this block serves. Amplitude
// takes the middle route, resolving only when a concrete technology corroborates it.
func TestParse_MarketingMeasurementAndSocialTooling(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{"looker studio", "Dashboards in Looker Studio.", []string{"looker-studio"}},
		{"google tag manager spelled out", "Tagging via Google Tag Manager.", []string{"google-tag-manager"}},
		{"hootsuite", "Scheduling in Hootsuite.", []string{"hootsuite"}},
		{"sprout social", "Reporting from Sprout Social.", []string{"sprout-social"}},
		{"contentful", "Publish through Contentful.", []string{"contentful"}},
		{"amplitude with corroboration", "Product analytics in Amplitude, events piped via Python.", []string{"amplitude"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.text)
			for _, w := range tc.want {
				if !slices.Contains(got, w) {
					t.Errorf("Parse(%q) = %v, want it to contain %q", tc.text, got, w)
				}
			}
		})
	}
}

// TestParse_ToolNamesThatAreOrdinaryWordsDoNotTag pins the collisions this block
// must never introduce. Each sentence is realistic marketing prose.
func TestParse_ToolNamesThatAreOrdinaryWordsDoNotTag(t *testing.T) {
	cases := []struct{ text, unwanted string }{
		{"Define the customer segment and its jobs to be done.", "segment"},
		{"Maintain a healthy content buffer of two weeks.", "buffer"},
		{"Applications reviewed now or later in the quarter.", "later"},
		{"The amplitude of seasonal demand shifts each quarter.", "amplitude"},
	}
	for _, tc := range cases {
		if got := Parse(tc.text); slices.Contains(got, tc.unwanted) {
			t.Errorf("Parse(%q) = %v, must not emit %q", tc.text, got, tc.unwanted)
		}
	}
}

// TestParse_MarketingDisciplines covers the disciplines themselves, so a search can
// combine "does technical SEO" with "uses Ahrefs". They are multi-word phrases,
// which the matcher already treats as strong — no gating needed. "SEM" is absent by
// design: it is a Portuguese and Spanish preposition ("sem experiência"), and this
// catalogue carries a large PT/ES population.
func TestParse_MarketingDisciplines(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{"technical seo", "You will lead technical SEO audits.", []string{"technical-seo"}},
		{"link building", "Own link building and digital PR.", []string{"link-building"}},
		{"paid social", "Scale our paid social programme.", []string{"paid-social"}},
		{"demand generation", "Run demand generation for EMEA.", []string{"demand-generation"}},
		{"lifecycle marketing", "Own lifecycle marketing end to end.", []string{"lifecycle-marketing"}},
		{"marketing automation", "Build marketing automation flows.", []string{"marketing-automation"}},
		{"generative engine optimization", "Lead generative engine optimization efforts.", []string{"generative-engine-optimization"}},
		{"answer engine optimization is the same canonical", "Lead answer engine optimization efforts.", []string{"generative-engine-optimization"}},
		{"content marketing", "Own content marketing strategy.", []string{"content-marketing"}},
		{"email marketing", "Own email marketing and deliverability.", []string{"email-marketing"}},
		{"influencer marketing", "Run influencer marketing campaigns.", []string{"influencer-marketing"}},
		{"copywriting", "Strong copywriting skills required.", []string{"copywriting"}},
		{"ppc", "Manage PPC campaigns across channels.", []string{"ppc"}},
		{"paid search is ppc", "Own paid search performance.", []string{"ppc"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.text)
			for _, w := range tc.want {
				if !slices.Contains(got, w) {
					t.Errorf("Parse(%q) = %v, want it to contain %q", tc.text, got, w)
				}
			}
		})
	}
}

// TestParse_SemIsNotAMarketingCanonical pins the deliberate omission: "sem" is a
// preposition in the Portuguese and Spanish postings this catalogue carries in bulk.
func TestParse_SemIsNotAMarketingCanonical(t *testing.T) {
	for _, text := range []string{
		"Profissional sem experiência prévia é bem-vindo.",
		"Puesto sem horario fijo.",
	} {
		if got := Parse(text); len(got) > 0 {
			t.Errorf("Parse(%q) = %v, want no canonicals", text, got)
		}
	}
}

// TestParse_GTMIsGoToMarket pins the homonym the right way round. In a job posting
// "GTM" is overwhelmingly go-to-market — "GTM strategy", "GTM motion" — while
// Google Tag Manager is spelled out or named as a container. Reading the bare
// abbreviation as the tag manager would mislabel the marketing corpus.
func TestParse_GTMIsGoToMarket(t *testing.T) {
	cases := []struct {
		name         string
		text         string
		want, unwant string
	}{
		{"gtm strategy", "You will own our GTM strategy end to end.", "go-to-market", "google-tag-manager"},
		{"gtm motion", "Shape the GTM motion for a new segment.", "go-to-market", "google-tag-manager"},
		{"spelled out go to market", "Own the go-to-market plan.", "go-to-market", "google-tag-manager"},
		{"bare abbreviation among tools is not the tag manager", "Tooling: GTM, GA4.", "", "google-tag-manager"},
		{"spelled out product", "Deploy tags through Google Tag Manager.", "google-tag-manager", "go-to-market"},
		{"container form", "Maintain the GTM container and its triggers.", "google-tag-manager", "go-to-market"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.text)
			if tc.want != "" && !slices.Contains(got, tc.want) {
				t.Errorf("Parse(%q) = %v, want it to contain %q", tc.text, got, tc.want)
			}
			if slices.Contains(got, tc.unwant) {
				t.Errorf("Parse(%q) = %v, must not contain %q", tc.text, got, tc.unwant)
			}
		})
	}
}

// TestParse_DisciplinePhrasesDoNotCorroborate pins the matcher rule this change
// adds. A phrase match is normally strong, and a strong match rescues the gated
// single-word canonicals. A marketing discipline is a concept, not a concrete
// technology, so it must tag without rescuing them — otherwise the "AI-powered"
// prose that saturates marketing postings tags the whole population with `ai`.
// A named product keeps corroborating, which is what recovers `seo`.
func TestParse_DisciplinePhrasesDoNotCorroborate(t *testing.T) {
	t.Run("discipline does not pull in the gated concept", func(t *testing.T) {
		got := Parse("AI-powered marketing automation for our sales team.")
		if !slices.Contains(got, "marketing-automation") {
			t.Errorf("Parse(...) = %v, want marketing-automation", got)
		}
		for _, unwanted := range []string{"ai", "automation"} {
			if slices.Contains(got, unwanted) {
				t.Errorf("Parse(...) = %v, must not contain %q", got, unwanted)
			}
		}
	})
	t.Run("product still corroborates", func(t *testing.T) {
		got := Parse("SEO Specialist — keyword research in Ahrefs and Semrush.")
		for _, want := range []string{"ahrefs", "semrush", "seo"} {
			if !slices.Contains(got, want) {
				t.Errorf("Parse(...) = %v, want %q", got, want)
			}
		}
	})
	t.Run("discipline stands alone", func(t *testing.T) {
		got := Parse("Own content marketing for the brand.")
		if !slices.Contains(got, "content-marketing") {
			t.Errorf("Parse(...) = %v, want content-marketing", got)
		}
	})
}

// TestParse_SalesAndSupportDoNotCorroborate mirrors
// TestParse_DisciplinePhrasesDoNotCorroborate for the sales/support phrases: they tag
// on their own but must not rescue a gated word that merely appears beside them.
func TestParse_SalesAndSupportDoNotCorroborate(t *testing.T) {
	t.Run("sales phrase does not pull in the gated concept", func(t *testing.T) {
		got := Parse("Manage our CRM as an account executive.")
		if !slices.Contains(got, "account-executive") {
			t.Errorf("Parse(...) = %v, want account-executive", got)
		}
		if slices.Contains(got, "crm") {
			t.Errorf("Parse(...) = %v, must not contain %q", got, "crm")
		}
	})
	t.Run("support phrase does not pull in the gated concept", func(t *testing.T) {
		got := Parse("Ticket resolution specialist troubleshooting via shell scripts the eng team wrote.")
		if !slices.Contains(got, "ticket-resolution") {
			t.Errorf("Parse(...) = %v, want ticket-resolution", got)
		}
		if slices.Contains(got, "shell") {
			t.Errorf("Parse(...) = %v, must not contain %q", got, "shell")
		}
	})
	t.Run("sales and support phrases stand alone", func(t *testing.T) {
		got := Parse("Business development lead driving cold outreach and pipeline management.")
		for _, want := range []string{"business-development", "cold-outreach", "pipeline-management"} {
			if !slices.Contains(got, want) {
				t.Errorf("Parse(...) = %v, want %q", got, want)
			}
		}
	})
	t.Run("lead generation does not pull in the gated concept", func(t *testing.T) {
		got := Parse("Lead generation using our CRM and analytics tools.")
		if !slices.Contains(got, "lead-generation") {
			t.Errorf("Parse(...) = %v, want lead-generation", got)
		}
		for _, unwanted := range []string{"crm", "analytics"} {
			if slices.Contains(got, unwanted) {
				t.Errorf("Parse(...) = %v, must not contain %q", got, unwanted)
			}
		}
	})
	t.Run("sales enablement does not pull in the gated concept", func(t *testing.T) {
		got := Parse("Sales enablement lead supporting reps with our CRM and analytics stack.")
		if !slices.Contains(got, "sales-enablement") {
			t.Errorf("Parse(...) = %v, want sales-enablement", got)
		}
		for _, unwanted := range []string{"crm", "analytics"} {
			if slices.Contains(got, unwanted) {
				t.Errorf("Parse(...) = %v, must not contain %q", got, unwanted)
			}
		}
	})
}

// TestParse_SupportSchedulingLegalPracticeTooling covers the nontech-derive-coverage
// wave: the tooling administrative, customer-support and immigration-practice
// postings actually name. See
// docs/superpowers/specs/2026-09-19-nontech-derive-coverage-design.md.
func TestParse_SupportSchedulingLegalPracticeTooling(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"freshdesk", "Support tickets are handled in Freshdesk.", "freshdesk"},
		{"calendly", "Interviews are booked via Calendly links.", "calendly"},
		// "clio" is gated (ambiguousWords: it collides with a common first name,
		// the Clio Awards, and the Renault Clio), so this case corroborates it
		// with another named tool — see TestParse_ClioNeedsCorroboration for the
		// gate itself.
		{"clio", "Manage client files and billing in Clio, synced with QuickBooks.", "clio"},
		{"uscis", "Coordinate with USCIS on pending petitions.", "uscis"},
		{"form i-129", "Prepare and file Form I-129 petitions.", "i-129"},
		{"form i-130", "Prepare and file Form I-130 petitions.", "i-130"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.in)
			if !slices.Contains(got, tc.want) {
				t.Errorf("Parse(%q) = %v, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestParse_ClioNeedsCorroboration guards the ambiguousWords gate on "clio": the
// word is at least as common a first name as "maya"/"lottie", and also names the
// Clio Awards and the Renault Clio, so a bare mention must not tag the legal
// canonical — the same treatment "houdini" and "maya" already carry.
func TestParse_ClioNeedsCorroboration(t *testing.T) {
	t.Run("clio the name does not tag alone", func(t *testing.T) {
		got := Parse("This role reports directly to Clio, our VP of Marketing.")
		if slices.Contains(got, "clio") {
			t.Errorf("Parse(...) = %v, must not contain %q", got, "clio")
		}
	})
	t.Run("clio the awards does not tag alone", func(t *testing.T) {
		got := Parse("Our campaigns have been recognized with a Clio Award.")
		if slices.Contains(got, "clio") {
			t.Errorf("Parse(...) = %v, must not contain %q", got, "clio")
		}
	})
	t.Run("clio corroborated by another named tool tags", func(t *testing.T) {
		got := Parse("Billing and intake run through Clio, integrated with QuickBooks.")
		if !slices.Contains(got, "clio") {
			t.Errorf("Parse(...) = %v, want %q", got, "clio")
		}
	})
}

// TestParse_MarketingSeparatorInsensitive checks that the separator rule the
// matcher already guarantees holds for the new multi-word canonicals too — the
// dictionary lists only the space form, so the hyphen and underscore spellings
// depend on that normalization.
func TestParse_MarketingSeparatorInsensitive(t *testing.T) {
	for _, form := range []string{"paid social", "paid-social", "paid_social"} {
		got := Parse("We run " + form + " campaigns.")
		if !slices.Contains(got, "paid-social") {
			t.Errorf("Parse(%q) = %v, want paid-social", form, got)
		}
	}
	for _, form := range []string{"demand generation", "demand-generation", "demand_generation"} {
		got := Parse("Own " + form + " for EMEA.")
		if !slices.Contains(got, "demand-generation") {
			t.Errorf("Parse(%q) = %v, want demand-generation", form, got)
		}
	}
}
