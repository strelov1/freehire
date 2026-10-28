# Interview Buddies Landing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a public landing page at `/features/interview-buddies` that explains the free, anonymous interview-buddies network and sends visitors to `/my/buddies`. Add a link to it in the footer.

**Architecture:**
- The page mirrors the shipped `/features/tailor` landing.
- The demo copy lives in `buddiesDemo.ts`. The looping hero animation `BuddiesHeroDemo.svelte` reads it, and the server render shows the finished state.
- `buddiesFaq.ts` feeds both the visible FAQ and the FAQPage JSON-LD.
- `BuddiesLandingView.svelte` holds the page body. The route file holds only the Seo block and the JSON-LD.

**Tech Stack:** SvelteKit 3, Svelte 5 runes, Tailwind through the design-system tokens, vitest. Files named `*.test.ts` run in node; files named `*.spec.ts` run in jsdom with `@testing-library/svelte`.

Spec: `docs/superpowers/specs/2026-10-28-buddies-landing-design.md`. Read its "Facts the copy may state" section. The page states nothing beyond that list.

Worktree: `/Users/i_strelov/Projects/hire-wt-buddies-landing`, branch `buddies-landing`. Run every command from `web/`. If vitest cannot find `.svelte-kit`, run `pnpm exec svelte-kit sync` first.

## Global Constraints

- **Copy limits.** No user counts, testimonials or claims we cannot prove. No competitor names. No Sparkles icon. Write "CV", never "résumé". English only.
- **Tailwind.** No arbitrary values, such as `text-[11px]` or `grid-cols-[…]`. The pre-commit design-system-tokens hook rejects them.
- **Baseline files.** The design-system adoption check can ask for a baseline update. If it does, run the command it prints and commit the file.
- **Formatting.** Never run prettier, because the repo has none.
- **Commit messages.** Never put backticks in a commit message.
- **Exports.** knip fails on any export that nothing outside tests imports.
- **Demo content.** Every handle, company and event in the demo is illustrative and labelled "example".
  - Event types are limited to `applied`, `interview` and `offer`. Never show a rejection, a withdrawal or an accepted offer.
  - Handles match `^[a-z]+-[a-z]+-\d{2,4}$` and use words from `internal/engage/buddy/handle.go`.
  - The companies are invented: Northwind, Globex and Initech.
- **The free CTA.** It is a plain link to `/my/buddies`. The `/my` layout already sends a signed-out visitor to sign in and back.
- **TailorHeroDemo.** Do not touch `TailorHeroDemo`, the tailor landing or anything outside the files this plan names.

---

### Task 1: Demo data and the hero demo

**Files:**
- Create: `web/src/lib/buddiesDemo.ts`, `web/src/lib/buddiesDemo.test.ts`
- Create: `web/src/lib/components/BuddiesHeroDemo.svelte`, `web/src/lib/components/BuddiesHeroDemo.spec.ts`

**Interfaces:**
- Produces:
  - `DEMO_FEED: { handle: string; role: string; type: 'applied' | 'interview' | 'offer'; company: string; day: string }[]`
  - `DEMO_INVITE: { to: string; body: string }`
  - `<BuddiesHeroDemo />`, which takes no props.

- [ ] **Step 1: Write the failing data test**

```ts
// web/src/lib/buddiesDemo.test.ts
import { describe, expect, it } from 'vitest';
import { DEMO_FEED, DEMO_INVITE } from './buddiesDemo';

// The landing must never show what the real feed hides: no rejection, no withdrawal,
// no accepted offer. And a demo handle must look like a real one.
describe('buddies demo copy', () => {
  it('uses only event types the real feed shows', () => {
    for (const e of DEMO_FEED) expect(['applied', 'interview', 'offer']).toContain(e.type);
  });

  it('uses handles shaped like the real ones', () => {
    for (const e of DEMO_FEED) expect(e.handle).toMatch(/^[a-z]+-[a-z]+-\d{2,4}$/);
    expect(DEMO_INVITE.to).toMatch(/^[a-z]+-[a-z]+-\d{2,4}$/);
  });

  it('invites someone who is in the feed, within the 500-character limit', () => {
    expect(DEMO_FEED.map((e) => e.handle)).toContain(DEMO_INVITE.to);
    expect(DEMO_INVITE.body.length).toBeGreaterThan(0);
    expect(DEMO_INVITE.body.length).toBeLessThanOrEqual(500);
  });
});
```

- [ ] **Step 2: Run it and check that it fails.** Run `pnpm vitest run src/lib/buddiesDemo.test.ts`. Expect FAIL, because the module is missing.

- [ ] **Step 3: Implement the data**

```ts
// web/src/lib/buddiesDemo.ts
// The illustrative feed the interview-buddies landing animates in its hero
// (BuddiesHeroDemo.svelte). Handles come from the real word banks
// (internal/engage/buddy/handle.go) and the companies are invented. Event types are the
// three the real feed shows; a rejection, a withdrawal or an accepted offer never
// appears there, so it never appears here either.

export const DEMO_FEED: {
  handle: string;
  role: string;
  type: 'applied' | 'interview' | 'offer';
  company: string;
  day: string;
}[] = [
  { handle: 'amber-heron-42', role: 'Backend Engineer', type: 'applied', company: 'Northwind', day: '3 days ago' },
  { handle: 'tidy-kestrel-318', role: 'Senior Backend Engineer', type: 'interview', company: 'Globex', day: 'yesterday' },
  { handle: 'brisk-osprey-77', role: 'Platform Engineer', type: 'applied', company: 'Globex', day: 'yesterday' },
  { handle: 'kind-reef-2041', role: 'Backend Engineer', type: 'offer', company: 'Initech', day: 'today' },
];

export const DEMO_INVITE = {
  to: 'tidy-kestrel-318',
  body: 'Hi! How was the Globex interview? Mine is on Friday.',
};
```

- [ ] **Step 4: Run it and check that it passes.** Run `pnpm vitest run src/lib/buddiesDemo.test.ts`. Expect PASS on 3 tests.

- [ ] **Step 5: Write the failing component spec**

```ts
// web/src/lib/components/BuddiesHeroDemo.spec.ts
import { render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import BuddiesHeroDemo from './BuddiesHeroDemo.svelte';

function mockMotion(reduce: boolean) {
  vi.stubGlobal('matchMedia', (q: string) => ({ matches: reduce && q.includes('reduce'), media: q }));
}

let observed: ((entries: { isIntersecting: boolean }[]) => void) | null = null;
vi.stubGlobal(
  'IntersectionObserver',
  class {
    constructor(cb: (entries: { isIntersecting: boolean }[]) => void) {
      observed = cb;
    }
    observe() {}
    disconnect() {}
  },
);

afterEach(() => {
  vi.useRealTimers();
  observed = null;
});

const entry = (handle: string) => screen.getByText(handle).closest('li') as HTMLElement;
const invite = () => screen.getByText(/How was the Globex interview/).closest('div') as HTMLElement;

describe('BuddiesHeroDemo', () => {
  it('shows every entry and the invitation when motion is reduced', () => {
    mockMotion(true);
    render(BuddiesHeroDemo);
    expect(entry('amber-heron-42').className).toContain('opacity-100');
    expect(entry('kind-reef-2041').className).toContain('opacity-100');
    expect(invite().className).not.toContain('invisible');
  });

  it('starts from one entry once visible, then adds the next', () => {
    vi.useFakeTimers();
    mockMotion(false);
    render(BuddiesHeroDemo);
    observed?.([{ isIntersecting: true }]);
    flushSync();
    expect(entry('amber-heron-42').className).toContain('opacity-100');
    expect(entry('tidy-kestrel-318').className).toContain('opacity-0');
    expect(invite().className).toContain('invisible');
    vi.advanceTimersByTime(1800); // one step, matching STEP_MS in the component
    flushSync();
    expect(entry('tidy-kestrel-318').className).toContain('opacity-100');
  });
});
```

- [ ] **Step 6: Run it and check that it fails.** Run `pnpm vitest run src/lib/components/BuddiesHeroDemo.spec.ts`. Expect FAIL, because the component is missing.

- [ ] **Step 7: Implement the component**

The rules:
- `step` counts the shown entries and runs from 1 to `FEED_LEN`.
- At `step === LAST`, where `LAST = FEED_LEN + 1`, the invitation shows as well.
- Entries list in chronological order. Each new one appears below the last, the way a chat appends. Every slot is always rendered and toggled by opacity, so the card height never changes.
- The server render and reduced motion show `step = LAST`.

```svelte
<!-- web/src/lib/components/BuddiesHeroDemo.svelte -->
<!-- The interview-buddies loop in miniature for the landing hero: other members' progress
     arriving in the feed, then an invitation to the one who just had the interview.
     Illustrative — the copy is DEMO_* in buddiesDemo.ts. Rendered on the server, and under
     reduced motion, as its finished state. Every slot is always laid out and only its
     opacity changes, so the card never changes height mid-loop. -->
<script lang="ts">
  import { DEMO_FEED, DEMO_INVITE } from '#lib/buddiesDemo.js';

  const STEP_MS = 1800;
  const HOLD_TICKS = 2;
  const FEED_LEN = DEMO_FEED.length;
  const LAST = FEED_LEN + 1;

  const EVENT_TEXT = {
    applied: 'applied to',
    interview: 'reached the interview at',
    offer: 'got an offer from',
  } as const;

  let step = $state(LAST);
  let root: HTMLElement | undefined = $state();

  $effect(() => {
    if (!root || window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) return;
    let timer: ReturnType<typeof setInterval> | undefined;
    let tick = 0;
    const start = () => {
      tick = 0;
      step = 1;
      timer = setInterval(() => {
        tick = (tick + 1) % (LAST + HOLD_TICKS);
        step = Math.min(tick + 1, LAST);
      }, STEP_MS);
    };
    const stop = () => {
      clearInterval(timer);
      timer = undefined;
      step = LAST;
    };
    const io = new IntersectionObserver((entries) => {
      const visible = entries.at(-1)?.isIntersecting;
      if (visible && !timer) start();
      else if (visible === false && timer) stop();
    });
    io.observe(root);
    return () => {
      io.disconnect();
      clearInterval(timer);
    };
  });
</script>

<figure bind:this={root} class="overflow-hidden rounded-xl border border-border bg-card shadow-sm">
  <figcaption
    class="flex items-center justify-between border-b border-border px-4 py-2.5 text-xs text-muted-foreground"
  >
    <span>freehire · Interview buddies</span>
    <span>example</span>
  </figcaption>
  <p class="sr-only">
    An example feed: {FEED_LEN} anonymous members applying to and interviewing at the same companies
    as you, then a message to {DEMO_INVITE.to} asking how their interview went.
  </p>
  <div aria-hidden="true" class="bg-background p-4">
    <p class="text-xs uppercase tracking-wide text-muted-foreground">Feed · similar to you</p>
    <ul class="mt-3 divide-y divide-border">
      {#each DEMO_FEED as e, i (e.handle)}
        <li class="py-2.5 transition-opacity duration-500 {i < step ? 'opacity-100' : 'opacity-0'}">
          <div class="flex items-baseline justify-between gap-3">
            <span class="font-mono text-xs font-medium">{e.handle}</span>
            <span class="text-xs text-muted-foreground">{e.day}</span>
          </div>
          <p class="mt-0.5 text-sm">
            <span class="text-muted-foreground">{e.role} ·</span>
            {EVENT_TEXT[e.type]}
            <span class="font-medium {e.type === 'applied' ? '' : 'text-success-strong'}">{e.company}</span>
          </p>
        </li>
      {/each}
    </ul>
    <div
      class="ml-auto mt-3 max-w-xs rounded-lg bg-secondary px-3 py-2 transition-opacity duration-500 {step === LAST
        ? 'opacity-100'
        : 'invisible opacity-0'}"
    >
      <p class="text-xs text-muted-foreground">To {DEMO_INVITE.to}</p>
      <p class="mt-0.5 text-sm">{DEMO_INVITE.body}</p>
      <p class="mt-1 text-right text-xs text-muted-foreground">sent</p>
    </div>
  </div>
</figure>
```

The `invite()` helper in the spec takes the closest `div` of the body text, which is the bubble itself. If the spec cannot find a class on it, add `data-testid="buddies-invite"` to the bubble and query that instead.

- [ ] **Step 8: Run the specs and checks**
  - Run `pnpm vitest run src/lib/buddiesDemo.test.ts src/lib/components/BuddiesHeroDemo.spec.ts`. Expect PASS.
  - Run eslint on the four files. Expect it to be clean.

- [ ] **Step 9: Commit.** Message: "Add the interview buddies hero demo".

---

### Task 2: The FAQ

**Files:**
- Create: `web/src/lib/buddiesFaq.ts`
- Create: `web/src/lib/buddiesFaq.test.ts`, a copy of `web/src/lib/tailorFaq.test.ts` with `TAILOR_FAQ` changed to `BUDDIES_FAQ`, the import path changed to `./buddiesFaq`, and the describe name changed.

**Interfaces:** Produces `BUDDIES_FAQ: FaqItem[]`. `FaqItem` comes from `./seo`.

- [ ] **Step 1: Write the test** (the copied file). Run it and expect FAIL, because the module is missing.

- [ ] **Step 2: Implement**

```ts
// Interview-buddies landing FAQ — the single source for both the visible section
// (BuddiesLandingView.svelte) and the FAQPage JSON-LD (routes/features/interview-buddies).
// Google requires the schema's answers to match the on-page text, so they share this.
// Every answer restates internal/engage/buddy/AGENTS.md and the interview-buddies spec;
// nothing here may promise more than those do.

import type { FaqItem } from './seo';

export const BUDDIES_FAQ: FaqItem[] = [
  {
    question: 'Will my employer or a recruiter see this?',
    answer:
      'No. Buddy pages are visible only to signed-in members of the network — not to search engines, not to recruiters, and not in the public Talent Network catalogue. You appear under a random handle, separate from any other name you use on freehire.',
  },
  {
    question: 'What exactly do other members see about me?',
    answer:
      'Your handle, your role titles, and facts like your category and country. In the feed, they see when you applied to a company and when you reached the interview or offer stage, by day. Never your name, email, employer, or anything you wrote in your CV — and never a rejection, a withdrawal, or the offer you accepted.',
  },
  {
    question: 'Can I keep one application out of the feed?',
    answer:
      'Yes. Every application has a switch to hide it from the buddy feed, and postings you pasted in privately never appear there at all.',
  },
  {
    question: "What if someone messages me in a way I don't like?",
    answer:
      'Block them: the conversation ends for good and you disappear from each other everywhere in the network. You can also report the conversation to our moderators. Declining an invitation is silent — the other person is not told.',
  },
  {
    question: 'Does it cost anything?',
    answer: 'No. Interview buddies is free for every signed-in account.',
  },
  {
    question: 'What happens if I leave?',
    answer:
      'You disappear from every list and feed, and your conversations close. If you come back later, you get the same handle back and old conversations can reopen.',
  },
  {
    question: 'Will I know when someone writes to me?',
    answer:
      'Yes. An unread message is announced by email, Telegram if you linked it, and in the app. The notice says only that a message is waiting — never what it says.',
  },
];
```

- [ ] **Step 3: Run it.** Run `pnpm vitest run src/lib/buddiesFaq.test.ts`. Expect PASS.
- [ ] **Step 4: Commit.** Message: "Add the interview buddies FAQ".

---

### Task 3: The landing view, the route and the footer link

**Files:**
- Create: `web/src/lib/components/BuddiesLandingView.svelte`, `web/src/lib/components/BuddiesLandingView.spec.ts`
- Create: `web/src/routes/features/interview-buddies/+page.svelte`
- Modify: `web/src/lib/components/Footer.svelte`. Add one link to the Features group, after Referrals.

**Interfaces:** Consumes `BuddiesHeroDemo` and `BUDDIES_FAQ`.

- [ ] **Step 1: Write the failing spec**

```ts
// web/src/lib/components/BuddiesLandingView.spec.ts
import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import BuddiesLandingView from './BuddiesLandingView.svelte';
import { BUDDIES_FAQ } from '#lib/buddiesFaq.js';

vi.mock('$app/paths', () => ({ resolve: (p: string) => p }));
vi.stubGlobal('matchMedia', (q: string) => ({ matches: q.includes('reduce'), media: q }));
vi.stubGlobal('IntersectionObserver', class { observe() {} disconnect() {} });

describe('BuddiesLandingView', () => {
  it('leads with the headline and sends every join button to /my/buddies', () => {
    render(BuddiesLandingView);
    expect(screen.getByRole('heading', { level: 1 }).textContent).toContain('Find people interviewing where you are.');
    const joins = screen.getAllByRole('link', { name: 'Join interview buddies' });
    expect(joins.length).toBeGreaterThan(0);
    for (const a of joins) expect(a.getAttribute('href')).toBe('/my/buddies');
  });

  it('renders every FAQ question', () => {
    render(BuddiesLandingView);
    for (const f of BUDDIES_FAQ) expect(screen.getByText(f.question)).toBeTruthy();
  });
});
```

Run `pnpm vitest run src/lib/components/BuddiesLandingView.spec.ts` and expect FAIL.

- [ ] **Step 2: Implement the view**

Mirror the structure and classes of `web/src/lib/components/TailorLandingView.svelte`. Read it first: `dot-grid` hero, `SectionLabel`, the fact strip, `NumberedGrid`, the two-column `grid gap-px … bg-border` panels, the FAQ `dl` with `sm:odd:last:col-span-2`, the closing CTA box. Use `Button` from `#lib/ui/index.js` with `href={resolve('/my/buddies')} variant="primary" size="lg"` and the label **Join interview buddies**. Take the copy verbatim from the spec's "Page structure":
- **Hero.** SectionLabel `interview buddies · free`. h1 "Find people interviewing where you are." The subhead from the spec. The Join button, then a plain `<a href="#how-it-works">` reading "See how it works ↓". Microcopy "Free · Anonymous · Leave any time". On the right, `<BuddiesHeroDemo />`.
- **Fact strip.** "Free for everyone", "Anonymous handle", "No recruiters, no crawlers", "You choose what shows".
- **How it works.** `id="how-it-works" class="scroll-mt-20 …"`, SectionLabel `how it works`, and a `NumberedGrid` with these steps:
  - `01 Join` — "One click. You get a random handle like amber-heron-42 — no name, no photo."
  - `02 Meet people like you` — "Members ranked by the same role, the same country and region, and the companies you both applied to in the last 90 days."
  - `03 Follow their progress` — "A feed of who applied where and who reached the interview or the offer, by day."
  - `04 Ask the one who's been there` — "Invite a member to chat with a first message of up to 500 characters. Declining is silent."
- **Visibility section.** SectionLabel `what others see`, h2 "Your progress is visible. You aren't." Two columns:
  - "Others see": the handle, role titles, category and country, applications and interview or offer moves with company and day. Show each item as a green chip (`border-success-strong/50 text-success-strong`).
  - "Nobody sees": the name, email or employer, anything written in the CV, rejections and withdrawals, the offer you accepted, postings pasted privately. Show each item as a muted chip (`border-border text-muted-foreground`).
  - Under the columns, the line "Hide any single application from the feed with one switch on the job in your tracker."
- **Control section.** SectionLabel `you stay in control`, h2 "Every exit is one click." Three cards in a `sm:grid-cols-3` bordered grid:
  - **Block**: "Ends the conversation for good and hides you from each other everywhere."
  - **Report**: "Sends the conversation to our moderators."
  - **Leave**: "Hides you and closes your chats until you come back — with the same handle."
- **FAQ.** SectionLabel `faq`, h2 "Questions people ask before joining." The `dl` comes from `BUDDIES_FAQ`.
- **Closing.** h2 "Stop interviewing alone.", the paragraph "Free, anonymous, and you can leave any time.", and the Join button.

- [ ] **Step 3: The route**

```svelte
<!-- web/src/routes/features/interview-buddies/+page.svelte -->
<script lang="ts">
  import { page } from '$app/state';
  import BuddiesLandingView from '#lib/components/BuddiesLandingView.svelte';
  import Seo from '#lib/components/Seo.svelte';
  import { BUDDIES_FAQ } from '#lib/buddiesFaq.js';
  import { faqPageJsonLd, jsonLdScript } from '#lib/seo.js';

  const canonical = $derived(`${page.url.origin}/features/interview-buddies`);
  // The FAQ block and this payload render from the same BUDDIES_FAQ array, so the
  // structured data can never disagree with what the page shows.
  const jsonLd = $derived(jsonLdScript([faqPageJsonLd(BUDDIES_FAQ)]));
</script>

<Seo
  title="Interview buddies — find people interviewing where you are | freehire"
  description="An anonymous, free peer network for job seekers: see who is applying to the same companies and how far they got, and message the person who just had the interview you're preparing for. No recruiters, no crawlers."
  {canonical}
/>

<svelte:head>
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- non-executable JSON-LD built by jsonLdScript, which escapes `<`; raw injection is the only way to emit a structured-data <script> -->
  {@html jsonLd}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-6">
  <BuddiesLandingView />
</div>
```

- [ ] **Step 4: Footer.** In `web/src/lib/components/Footer.svelte`, add this line right after the Referrals line in the Features group:

```ts
        { label: 'Interview buddies', href: resolve('/features/interview-buddies') },
```

Then check whether any sitemap or static list of feature pages also needs the new route. Run `grep -rn "features/ghost-jobs" web/src --include=*.ts --include=*.svelte | grep -v routes/features`. Add the new page wherever the existing feature landings are listed, for example a sitemap static-pages list. Report each place you change.

- [ ] **Step 5: Verify**
  - Run `pnpm vitest run src/lib/components/BuddiesLandingView.spec.ts src/lib/components/Footer.test.ts`.
  - Run `pnpm check` and expect 0 errors.
  - Run eslint on the changed files.
  - Run the full suite with `pnpm vitest run`.
- [ ] **Step 6: Commit.** Message: "Add the interview buddies landing and link it from the footer".

---

### Task 4: Browser pass (controller)

- [ ] Start the dev server against prod:
  ```bash
  VITE_API_URL=https://freehire.me API_INTERNAL_URL=https://freehire.me pnpm exec vite dev --port 5192
  ```
- [ ] Screenshot `/features/interview-buddies` at 1440 and 390 widths with the Playwright script in `/tmp/tailor-shots/shot.mjs`. Point it at the new URL first.
- [ ] Check four things:
  - The demo loops, and the card keeps its height while it does.
  - Nothing scrolls sideways.
  - The FAQ grid has no empty cell.
  - The footer shows the link.
