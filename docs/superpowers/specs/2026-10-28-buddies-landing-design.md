# Interview buddies landing — design

Page: `/features/interview-buddies`. It gets a new route, a view component, an FAQ module
and a footer link.

## Goal

Interview buddies has shipped (`internal/engage/buddy`, `/my/buddies`), but no public page
explains it. Because the network is members-only, a visitor who is not signed in has no
way to learn it exists. The landing explains the feature and sends people to
`/my/buddies` to join.

It follows the `/features/tailor` landing: the same section rhythm, a live demo in the
hero, a fact strip and a FAQ that also feeds the FAQPage JSON-LD.

## Facts the copy may state

Each fact below comes from `internal/engage/buddy/AGENTS.md` or from the interview-buddies
spec. The page states nothing beyond this list.

- **Price.** Free for every signed-in account. There is no plan gate.
- **Joining.** Membership is opt-in. A member joins, gets a random handle shaped like
  `amber-heron-42`, and can leave at any time. A member who rejoins gets the same handle
  back.
- **The card.** It shows only dictionary-resolved values, numbers, dates and role titles
  verbatim, as the member wrote them (a title can name an employer, so the page tells
  members to edit it before joining). It never shows the person's name, email, username or
  any other free text from the CV. The buddy handle is separate from the Talent Network handle.
- **Who can see it.** Only signed-in members see buddy pages, and any account can join, a
  recruiter included. No crawler and not the public Talent Network catalogue can see them. They are marked non-indexable and
  are not in the sitemap.
- **Similar members.** They are ranked by shared category, shared country and region, and
  companies both people applied to in the last 90 days. Each entry shows the companies
  the two members share.
- **The feed.** It shows applications and moves into interview or offer, together with
  the company, the job title and the day. It never shows rejections, withdrawals,
  expiries or an accepted offer. Postings the member pasted privately never appear. A
  member can hide any single application from the feed.
- **Invitations.** An invitation is a first message of 1 to 500 characters. A member may
  send at most 10 invitations in any 24 hours. There is one conversation per pair.
  Declining is silent.
- **Chat.** It happens in the app. There are no contact fields; members swap contacts
  themselves if they choose to.
- **Notices.** An unread message is announced by email, Telegram and the in-app
  notification centre. A notice never carries message text or account details.
- **Safety.** A block ends contact for good and hides both members from each other
  everywhere. A report goes to the moderators. Leaving closes the member's conversations
  until they rejoin.

## Constraints

- No user counts, testimonials or other claims we cannot prove.
- No competitor names.
- No Sparkles icon.
- "CV", never "résumé". English copy only.
- No Tailwind arbitrary values. A pre-commit hook rejects them.
- Every handle, company and event in the demo is illustrative and labelled "example". The
  demo uses invented companies (Northwind, Globex, Initech) and handles generated from the
  real word banks.
- The demo never shows an event type that the real feed hides. It shows no rejection and
  no "accepted".

## Page structure

1. **Hero.**
   - Label: `interview buddies · free`.
   - Headline: "Find people interviewing where you are."
   - Subhead: "Join anonymously, see who's applying to the same companies and how far they
     got, and message the one who just had the interview you're preparing for."
   - CTA: **Join interview buddies**, which goes to `/my/buddies`. The `/my` shell sends a
     signed-out visitor to sign in. Secondary: "See how it works ↓".
   - Microcopy: "Free · Anonymous · Leave any time".
   - On the right, the live demo described below.
2. **Fact strip.** Four items: "Free for everyone", "Anonymous handle", "Members only, no
   crawlers" and "You choose what shows".
3. **How it works.** Four steps in a `NumberedGrid`:
   - Join, and get a random handle.
   - See members like you: same role, same place, same companies in the last 90 days.
   - Follow their progress: applied, interview, offer, by day.
   - Invite one to chat with a first message of up to 500 characters.
4. **What others see, and what nobody sees.** Two columns.
   - "Others see": your handle, role titles as written, category, seniority, skills and country, applications and
     interview or offer moves with company and day.
   - "Nobody sees": your name or email, anything else in your CV,
     rejections and withdrawals, the offer you accepted, postings you pasted privately.
   - One line underneath: "Hide any single application from the feed with one switch on the
     job in your tracker."
5. **You stay in control.** Three cards:
   - Block: it ends contact for good and hides you both everywhere.
   - Report: it goes to our moderators.
   - Leave: it hides you and closes your chats until you come back.
6. **FAQ.** It comes from `BUDDIES_FAQ` and feeds both the page and the JSON-LD. Questions:
   - Will my employer or a recruiter see this?
   - What exactly do other members see about me?
   - Can I keep one application out of the feed?
   - What if someone messages me in a way I don't like?
   - Does it cost anything?
   - What happens if I leave?
   - Will I know when someone writes to me?
7. **Closing CTA.** "Stop interviewing alone." with the same Join button.
8. **Footer.** Add `{ label: 'Interview buddies', href: resolve('/features/interview-buddies') }`
   to the Features column in `Footer.svelte`, after Referrals.

## Hero demo — `BuddiesHeroDemo.svelte` (new)

The demo is a looping animation built the same way as `TailorHeroDemo`.

- **Layout.** A card captioned "freehire · Interview buddies" with the label "example".
  - Body: a "Feed" list of up to four entries. Each entry shows a handle, a role title, the
    event ("applied to Northwind", "reached the interview at Globex", "got an offer from
    Initech") and a day such as "today" or "2 days ago".
  - Under the feed, an invitation bubble from the viewer to one handle: "Hi! How was the
    Globex onsite? I have mine on Friday." It carries a "sent" mark.
- **Loop.**
  - Entries appear one at a time, newest on top, every 1800 ms.
  - After the last entry the invitation bubble appears.
  - The final state holds for two ticks, then the loop resets.
  - The loop runs only while the demo is on screen, using IntersectionObserver.
- **Server render and reduced motion.** Both show the final state: every entry and the
  bubble.
- **Stable height.** Every entry slot and the bubble are always rendered and toggled with
  opacity or `invisible`, so the card never jumps. This avoids the bug found on the tailor
  demo.
- **Accessibility.** The demo is `aria-hidden`, and a sr-only sentence describes what it
  shows.
- **Data.** Content lives in `web/src/lib/buddiesDemo.ts`.
  - A unit test checks two things there: every event type is one of `applied`, `interview`
    or `offer`, and every handle matches the real handle shape
    `^[a-z]+-[a-z]+-\d{2,4}$`.

## Files

- `web/src/lib/buddiesDemo.ts` and `web/src/lib/buddiesDemo.test.ts`: new.
- `web/src/lib/components/BuddiesHeroDemo.svelte` and `BuddiesHeroDemo.spec.ts`: new.
- `web/src/lib/buddiesFaq.ts` and `buddiesFaq.test.ts`: new, mirroring `tailorFaq` and its test.
- `web/src/lib/components/BuddiesLandingView.svelte` and `BuddiesLandingView.spec.ts`: new.
- `web/src/routes/features/interview-buddies/+page.svelte`: new. It holds the Seo block and
  the JSON-LD, the same way the tailor page does.
- `web/src/lib/components/Footer.svelte`: one new link.

## Testing

- Unit tests for the demo data and the FAQ.
- Component specs:
  - The demo shows its final state when motion is reduced and starts from one entry once
    it is on screen.
  - The landing renders the h1, the Join link to `/my/buddies` and the FAQ.
- `pnpm check` with 0 errors, eslint clean, and the full `pnpm vitest run` passing.
- A browser pass at 1440 and 390 widths.
