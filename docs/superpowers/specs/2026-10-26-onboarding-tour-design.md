# Onboarding as a tour of what the site does

Approved 2026-10-26 against a clickable prototype (spike, verdict VALIDATED: every step
is assembled from components the site already ships).

## Why

User feedback (2026-10): "There is no sign-in button. The app wouldn't let me see the
job until I uploaded my CV and answered questions. I'd prefer onboarding to start right
after sign-up."

Measured against the code, the eight-step wizard collects answers whose payoff the
candidate never sees, and some feed nothing at all:

| Step | Read by |
|---|---|
| CV, specialization, level, skills, location | feed filters, match score, tailoring, auto-apply |
| links, years of experience | CV, auto-apply, the match's hard constraints |
| desired salary | auto-apply form answers only |
| current income, search stage | nothing |
| challenge | nothing yet — 10,554 answers on prod: 44% "getting recruiters to reply", 24% "working abroad", 9% English, 7% technical interviews |

The wizard also never turns on an alert, so a candidate who described what they want
gets no mail about it.

## The tour — seven steps, all skippable

1. **Start with your CV.** The existing CV upload (`CvStep`: PDF only, the LinkedIn
   "Save to PDF" hint). A parsed CV pre-fills step 2. "Skip — I'll pick by hand".
2. **What are you looking for?** One screen: specialization, level, skills, location.
   - Specialization: search that tolerates typos (`fuzzyMatch`) with a clear button;
     twelve pills shown, technical categories first (`vocab.TechCategories`, generated
     into the web contracts), each half by live job count; "Show all specializations".
   - Level: `SeniorityPills`.
   - Skills: `RemoteSearchSelect` with typo-tolerant matching; the list closes after a
     pick so the new chip is visible; a clear button in the field.
   - Location: `LocationPreferencesFields`, with room beneath it for its suggestions.
   - "Filled in from your CV" note when step 1 parsed one.
   - Live count of matching open jobs beside Continue (`/jobs/search`, `limit=1`, over
     `filtersFromProfile`).
   - Saves the search profile as today (`profileStore.save` + `syncProfileAlert`).
3. **Where should we send new jobs?** Three channels for the profile-derived alert:
   email (switch — the `ProfileAlertToggle` behaviour), Telegram (Connect → the bot
   link flow, then subscribe), push (only the iPhone app registers a device → "Get the
   app").
4. **Search jobs from your AI assistant with MCP.** The `/mcp` setup, extracted into a shared
   `McpSetup` component used by both pages. In the tour it is compact, opens on the
   account connector, and lifts the selected host's one-click install (Claude, Cursor)
   above the picker.
5. **What's hardest about your job search right now?** Several answers (`just_starting`
   stands alone), stored as `candidate_survey.biggest_challenges`. Ten answers: the five
   old ones, reworded, plus five the free-text "other" notes kept asking for (job fit,
   remote or flexible work, visa sponsorship, first job, just starting). A "What helps"
   panel beside the list shows, for the picked answers together, the features that address
   them; before an answer, a drawn sprout. "Something else" opens a note across both
   columns.
6. **Tailoring CV** (Pro). Before/after example, four points on how it
   works, link to `/features/tailor`.
7. **Fill any application from the side panel** — the browser extension; installing it is
   free, form filling is Pro. A sample queue, four points, link to
   `/features/extension`, "See Pro plans", and "Search, alerts and the AI connector stay
   free". Finish → "Go to my feed".

Content column 672px (`max-w-2xl`); Continue is the `cta` button variant.

Removed from onboarding: profile links, years of experience, desired salary, current
income, search stage. Their stores stay; auto-apply asks for what it is missing.

## Shown once, asking only what is missing

Migration 0207 re-asks every account once (it nulls `onboarding_completed_at`); the tour
skips any data step already answered and pre-fills the rest, and an account with nothing
missing is completed without a screen.


The layout routes any account with `onboarding_completed_at IS NULL` to `/onboarding`
on every visit until it is finished or closed with X — the "wouldn't let me see the
job" complaint. New rule: the tour marks the account complete when it is SHOWN. The
redirect stays (it starts the tour after sign-up) but fires at most once per account.

## Sign in in the header on phones

`HeaderProfileMenu` hides its container below `sm`, so a signed-out phone visitor has no
sign-in control outside the menu, and on desktop it is an unlabelled icon. Signed out,
render a labelled "Sign in" button at every width.

## Known copy fix

`McpSetup`'s account-connector description begins "Everything above", but only one
description is ever visible. Reword it to stand alone.
