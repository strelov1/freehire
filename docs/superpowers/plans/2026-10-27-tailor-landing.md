# Tailor Landing Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn `/features/tailor` into a sales page for Pro: a live before-and-after demo, the price, a comparison table, a founder note and an FAQ built around buyer objections.

**Architecture:** The demo copy lives in one module, `tailorDemo.ts`. The hero animation (`TailorHeroDemo.svelte`) and the page's before-and-after section both read from it. The Pro price is loaded on the server in a new `+page.server.ts` and passed to a small `ProCheckoutButton.svelte`, which starts checkout the same way `/pricing` does. The list of Pro perks moves out of the pricing page into `proPerks.ts`, so both pages show the same list. `TailorLandingView.svelte` is rewritten section by section.

**Tech Stack:** SvelteKit 3, Svelte 5 runes, Tailwind, the `freehire-design-system` primitives (through `#lib/ui/index.js`), vitest (`*.test.ts` runs in node, `*.spec.ts` runs `@testing-library/svelte` in jsdom).

Spec: `docs/superpowers/specs/2026-10-27-tailor-landing-design.md`.

Worktree: `/Users/i_strelov/Projects/hire-wt-tailor-landing`, branch `tailor-landing-redesign`. Run every command from `web/` inside it.

## Global Constraints

- No testimonials, user counts or claims we cannot prove. Figures allowed: 3.3M+ live jobs, 3.2k GitHub stars, open source (MIT), 9 templates.
- No competitor names anywhere, in copy or in code.
- No Sparkles icon. Mark AI in words.
- Say "CV", never "résumé". All copy is in English.
- Pro is $5 a month, cancel any time, with the 14-day no-questions refund /pricing already offers. Quote that refund exactly as /pricing states it. The price shown always comes from the backend's plan matrix, never from a hardcoded number.
- `TailorWorkspacePreview.svelte` stays unchanged, because the onboarding tour uses it.
- Every demo number and bullet is illustrative and labelled "example". An "after" bullet may only restate facts its "before" bullet already holds. The demo must not show tailoring that adds facts.
- The tailor page's server load must not read cookies. A shared cache keys the HTML on the URL alone.
- There is no prettier in the repo. Never run `npx prettier`.
- knip fails on any export nothing imports. Export only what another file uses.

---

### Task 1: The demo copy and the keyword highlighter

**Files:**
- Create: `web/src/lib/tailorDemo.ts`
- Test: `web/src/lib/tailorDemo.test.ts`

**Interfaces:**
- Produces:
  - `DEMO_ROLE: string`
  - `DEMO_REQUIREMENTS: { id: string; label: string }[]`
  - `DEMO_BULLETS: { req: string; before: string; after: string; keywords: string[] }[]`
  - `DEMO_ASKED: string`, the id of the requirement the agent asks about
  - `DEMO_SCORES: number[]`, with length `DEMO_BULLETS.length + 1`
  - `highlight(text: string, words: string[]): { text: string; hit: boolean }[]`

- [ ] **Step 1: Write the failing test**

```ts
// web/src/lib/tailorDemo.test.ts
import { describe, expect, it } from 'vitest';
import { DEMO_ASKED, DEMO_BULLETS, DEMO_REQUIREMENTS, DEMO_SCORES, highlight } from './tailorDemo';

describe('highlight', () => {
  it('marks every occurrence of each word, case-insensitively', () => {
    expect(highlight('Built the Kafka pipeline; kafka at scale', ['kafka'])).toEqual([
      { text: 'Built the ', hit: false },
      { text: 'Kafka', hit: true },
      { text: ' pipeline; ', hit: false },
      { text: 'kafka', hit: true },
      { text: ' at scale', hit: false },
    ]);
  });

  it('returns the text whole when nothing matches', () => {
    expect(highlight('Code reviews', ['Rust'])).toEqual([{ text: 'Code reviews', hit: false }]);
  });

  it('treats regex characters in a word literally', () => {
    expect(highlight('C++ and C', ['C++'])).toEqual([
      { text: 'C++', hit: true },
      { text: ' and C', hit: false },
    ]);
  });
});

describe('demo copy', () => {
  // The page argues that tailoring adds nothing you did not do. The example must not contradict it.
  it('has one score per step, rising', () => {
    expect(DEMO_SCORES).toHaveLength(DEMO_BULLETS.length + 1);
    for (let i = 1; i < DEMO_SCORES.length; i++) expect(DEMO_SCORES[i]).toBeGreaterThan(DEMO_SCORES[i - 1]);
  });

  it('ties every bullet and the asked requirement to a listed requirement', () => {
    const ids = new Set(DEMO_REQUIREMENTS.map((r) => r.id));
    for (const b of DEMO_BULLETS) expect(ids.has(b.req), b.req).toBe(true);
    expect(ids.has(DEMO_ASKED)).toBe(true);
    expect(DEMO_BULLETS.some((b) => b.req === DEMO_ASKED)).toBe(false);
  });

  it('highlights only words the after line actually contains', () => {
    for (const b of DEMO_BULLETS) {
      for (const k of b.keywords) expect(b.after.toLowerCase(), k).toContain(k.toLowerCase());
    }
  });
});
```

- [ ] **Step 2: Run the test and check that it fails**

Run: `pnpm vitest run src/lib/tailorDemo.test.ts`
Expected: FAIL, because `./tailorDemo` does not exist.

- [ ] **Step 3: Implement**

```ts
// web/src/lib/tailorDemo.ts
// The illustrative example the tailoring landing shows twice: animated in the hero
// (TailorHeroDemo.svelte) and laid out as before/after lines further down. One copy, so
// the two can never tell different stories. Every "after" line restates a fact its
// "before" line already holds — the page sells "invent nothing", and its own example
// must not invent either.

export const DEMO_ROLE = 'Senior Backend Engineer · Payments';

export const DEMO_REQUIREMENTS = [
  { id: 'kafka', label: 'Kafka' },
  { id: 'payments', label: 'Payment systems' },
  { id: 'mentoring', label: 'Mentoring' },
  { id: 'k8s', label: 'Kubernetes operators' },
];

export const DEMO_BULLETS = [
  {
    req: 'kafka',
    before: 'Backend work for the checkout team (Go, Kafka).',
    after: 'Built the Kafka event pipeline behind checkout, in Go.',
    keywords: ['Kafka'],
  },
  {
    req: 'payments',
    before: 'Various backend tasks, incl. ledger migration to Postgres.',
    after: 'Migrated the payment ledger to Postgres.',
    keywords: ['payment'],
  },
  {
    req: 'mentoring',
    before: 'Code reviews, onboarding new hires.',
    after: 'Mentored new engineers through onboarding and code review.',
    keywords: ['Mentored'],
  },
];

// Nothing in the example history supports it, so the agent asks instead of writing it.
export const DEMO_ASKED = 'k8s';

// Job match before any edit, then after each rewritten bullet.
export const DEMO_SCORES = [54, 68, 79, 86];

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

/** Splits text into runs, flagging the ones that match any of the words (case-insensitive). */
export function highlight(text: string, words: string[]): { text: string; hit: boolean }[] {
  if (words.length === 0) return [{ text, hit: false }];
  const lower = new Set(words.map((w) => w.toLowerCase()));
  const re = new RegExp(`(${words.map(escape).join('|')})`, 'gi');
  return text
    .split(re)
    .filter((part) => part !== '')
    .map((part) => ({ text: part, hit: lower.has(part.toLowerCase()) }));
}
```

- [ ] **Step 4: Run the test and check that it passes**

Run: `pnpm vitest run src/lib/tailorDemo.test.ts`
Expected: PASS (6 tests).

- [ ] **Step 5: Commit**

```bash
git add src/lib/tailorDemo.ts src/lib/tailorDemo.test.ts
git commit -m "Add the tailoring landing's example copy and highlighter"
```

---

### Task 2: The hero demo component

**Files:**
- Create: `web/src/lib/components/TailorHeroDemo.svelte`
- Test: `web/src/lib/components/TailorHeroDemo.spec.ts`

**Interfaces:**
- Consumes: everything Task 1 produces except `highlight`.
- Produces: `<TailorHeroDemo />`, which takes no props.

Behaviour: the server render and reduced motion both show the final state, with score 86, every bullet rewritten and the asked note visible. With motion allowed, the loop starts when the figure scrolls into view: it resets to step 0, then advances one step every 1800 ms, holds the final state for two ticks and starts over. It stops, back on the final state, when the figure leaves the view.

- [ ] **Step 1: Write the failing test**

```ts
// web/src/lib/components/TailorHeroDemo.spec.ts
import { render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import TailorHeroDemo from './TailorHeroDemo.svelte';

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

describe('TailorHeroDemo', () => {
  it('shows the finished example when motion is reduced', () => {
    mockMotion(true);
    render(TailorHeroDemo);
    expect(screen.getByText('86')).toBeTruthy();
    expect(screen.getByText('Migrated the payment ledger to Postgres.')).toBeTruthy();
    expect(screen.getByText(/asked you first/)).toBeTruthy();
  });

  it('restarts from the first score once visible, then climbs', () => {
    vi.useFakeTimers();
    mockMotion(false);
    render(TailorHeroDemo);
    observed?.([{ isIntersecting: true }]);
    flushSync();
    expect(screen.getByText('54')).toBeTruthy();
    vi.advanceTimersByTime(1800); // one step — keep in step with STEP_MS in the component
    flushSync();
    expect(screen.getByText('68')).toBeTruthy();
  });
});
```

- [ ] **Step 2: Run the test and check that it fails**

Run: `pnpm vitest run src/lib/components/TailorHeroDemo.spec.ts`
Expected: FAIL, because the component does not exist.

- [ ] **Step 3: Implement**

```svelte
<!-- web/src/lib/components/TailorHeroDemo.svelte -->
<!-- The tailoring loop in miniature for the landing hero: bullets rewritten one at a
     time toward a vacancy while the job match climbs, and one requirement left as a
     question rather than written in. Illustrative — the copy is DEMO_* in tailorDemo.ts.
     Rendered on the server, and under reduced motion, as its finished state. -->
<script lang="ts">
  import { fade } from 'svelte/transition';
  import {
    DEMO_ASKED,
    DEMO_BULLETS,
    DEMO_REQUIREMENTS,
    DEMO_ROLE,
    DEMO_SCORES,
  } from '#lib/tailorDemo.js';

  const STEP_MS = 1800;
  const HOLD_TICKS = 2;
  const LAST = DEMO_BULLETS.length;

  let step = $state(LAST);
  let root: HTMLElement | undefined = $state();

  const score = $derived(DEMO_SCORES[step]);
  const met = $derived(new Set(DEMO_BULLETS.slice(0, step).map((b) => b.req)));

  function chipClass(id: string): string {
    if (met.has(id)) return 'border-success-strong/50 text-success-strong';
    if (id === DEMO_ASKED && step === LAST) return 'border-warning/50 text-warning-strong';
    return 'border-border text-muted-foreground';
  }

  $effect(() => {
    if (!root || window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) return;
    let timer: ReturnType<typeof setInterval> | undefined;
    let tick = 0;
    const start = () => {
      tick = 0;
      step = 0;
      timer = setInterval(() => {
        tick = (tick + 1) % (LAST + 1 + HOLD_TICKS);
        step = Math.min(tick, LAST);
      }, STEP_MS);
    };
    const stop = () => {
      clearInterval(timer);
      timer = undefined;
      step = LAST;
    };
    const io = new IntersectionObserver(([entry]) => {
      if (entry.isIntersecting && !timer) start();
      else if (!entry.isIntersecting && timer) stop();
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
    <span>freehire · Tailoring</span>
    <span>example</span>
  </figcaption>
  <p class="sr-only">
    An example: the agent rewrites three CV bullets toward a backend payments role, the job match
    rises from {DEMO_SCORES[0]} to {DEMO_SCORES[LAST]}, and it asks about Kubernetes operators
    instead of writing them in.
  </p>
  <div aria-hidden="true" class="grid gap-px bg-border sm:grid-cols-[0.8fr_1.2fr]">
    <div class="bg-background p-4">
      <p class="text-[11px] uppercase tracking-wide text-muted-foreground">Vacancy</p>
      <p class="mt-1 text-sm font-semibold">{DEMO_ROLE}</p>
      <ul class="mt-3 flex flex-wrap gap-1.5">
        {#each DEMO_REQUIREMENTS as r (r.id)}
          <li class="rounded-md border px-2 py-0.5 font-mono text-[11px] transition-colors duration-500 {chipClass(r.id)}">
            {r.label}
          </li>
        {/each}
      </ul>
      {#if step === LAST}
        <p in:fade={{ duration: 300 }} class="mt-3 text-[11px] text-warning-strong">
          Kubernetes operators: not in your history — asked you first.
        </p>
      {/if}
    </div>
    <div class="relative bg-background p-4">
      <div class="absolute right-4 top-4 text-right">
        <p class="text-3xl font-semibold tabular-nums">{score}</p>
        <p class="text-[11px] text-muted-foreground">job match</p>
      </div>
      <p class="text-[11px] uppercase tracking-wide text-muted-foreground">Your CV</p>
      <ul class="mt-3 space-y-2 pr-20 text-[12px] leading-snug">
        {#each DEMO_BULLETS as b, i (b.req)}
          <li>
            {#key i < step}
              <span
                in:fade={{ duration: 400 }}
                class={i < step ? 'rounded bg-brand/10 text-foreground' : 'text-muted-foreground'}
              >
                {i < step ? b.after : b.before}
              </span>
            {/key}
          </li>
        {/each}
      </ul>
    </div>
  </div>
</figure>
```

- [ ] **Step 4: Run the test and check that it passes**

Run: `pnpm vitest run src/lib/components/TailorHeroDemo.spec.ts`
Expected: PASS (2 tests). If `getByText('86')` finds two elements, the sr-only sentence also contains "86". In that case query the score with `getAllByText('86')` and check that one of the elements is the `<p>` with class `tabular-nums`.

- [ ] **Step 5: Commit**

```bash
git add src/lib/components/TailorHeroDemo.svelte src/lib/components/TailorHeroDemo.spec.ts
git commit -m "Add the tailoring hero demo"
```

---

### Task 3: The Pro price, perks and checkout button

**Files:**
- Create: `web/src/lib/proPrice.ts`, `web/src/lib/proPrice.test.ts`
- Create: `web/src/lib/proPerks.ts`
- Modify: `web/src/routes/pricing/+page.svelte`. Replace the inline `PAID_FEATURES` array (around lines 97-103) with an import of `PRO_PERKS`, and use `PRO_PERKS` where `PAID_FEATURES` was used.
- Create: `web/src/lib/components/ProCheckoutButton.svelte`, `web/src/lib/components/ProCheckoutButton.spec.ts`

**Interfaces:**
- Produces:
  - `proMonthlyPrice(plans: PlansMatrix | null): PublicPrice | null`
  - `priceLabel(p: PublicPrice): string`, which gives `"$5"` for 500 USD cents
  - `PRO_PERKS: { text: string; href: string }[]`
  - `<ProCheckoutButton price={PublicPrice | null} size?: 'lg' | 'default' />`

- [ ] **Step 1: Write the failing unit test**

```ts
// web/src/lib/proPrice.test.ts
import { describe, expect, it } from 'vitest';
import type { PlansMatrix, PublicPrice } from './types';
import { priceLabel, proMonthlyPrice } from './proPrice';

const price = (over: Partial<PublicPrice>): PublicPrice =>
  ({ id: 'p', amount_cents: 500, currency: 'usd', interval: 'month', default: true, tier: 'pro', ...over }) as PublicPrice;

describe('proMonthlyPrice', () => {
  it('picks the Pro monthly price', () => {
    const plans = { prices: [price({ id: 'u', tier: 'ultra' }), price({ id: 'y', interval: 'year' }), price({ id: 'm' })] } as PlansMatrix;
    expect(proMonthlyPrice(plans)?.id).toBe('m');
  });
  it('is null without a matrix or a monthly Pro price', () => {
    expect(proMonthlyPrice(null)).toBeNull();
    expect(proMonthlyPrice({ prices: [price({ interval: 'year' })] } as PlansMatrix)).toBeNull();
  });
});

describe('priceLabel', () => {
  it('drops zero cents', () => {
    expect(priceLabel(price({ amount_cents: 500 }))).toBe('$5');
  });
  it('keeps real cents', () => {
    expect(priceLabel(price({ amount_cents: 499 }))).toBe('$4.99');
  });
});
```

- [ ] **Step 2: Run it and check that it fails**

Run: `pnpm vitest run src/lib/proPrice.test.ts`
Expected: FAIL, because the module does not exist.

- [ ] **Step 3: Implement `proPrice.ts` and `proPerks.ts`**

```ts
// web/src/lib/proPrice.ts
// The Pro monthly price as the backend's plan matrix states it — the one source both
// /pricing and the feature landings quote, so no page carries its own number.
import { formatMinorUnits } from './money';
import type { PlansMatrix, PublicPrice } from './types';

export function proMonthlyPrice(plans: PlansMatrix | null): PublicPrice | null {
  return plans?.prices.find((p) => p.tier === 'pro' && p.interval === 'month') ?? null;
}

/** "$5" rather than "$5.00": a whole price reads as one on a call to action. */
export function priceLabel(p: PublicPrice): string {
  return formatMinorUnits(p.amount_cents, p.currency).replace(/[.,]00$/, '');
}
```

```ts
// web/src/lib/proPerks.ts
// What a paid plan adds, shown on /pricing and on the tailoring landing. The daily
// amounts come from the backend's plan matrix, so nothing here quotes a number.
import { resolve } from '$app/paths';

export const PRO_PERKS: { text: string; href: string }[] = [
  { text: 'CV tailoring for each vacancy, with cover letters', href: resolve('/features/tailor') },
  { text: 'AI assistant', href: resolve('/features/tailor') },
  { text: 'Chrome and Firefox extension that fills applications for you', href: resolve('/features/extension') },
  { text: 'Auto-apply in the background, within a daily limit', href: resolve('/features/extension') },
];
```

In `routes/pricing/+page.svelte`, delete the `PAID_FEATURES` declaration and its comment. Add `import { PRO_PERKS } from '#lib/proPerks.js';` and replace every `PAID_FEATURES` reference with `PRO_PERKS`. Keep the `Perk` type only if `FREE_FEATURES` still uses it.

- [ ] **Step 4: Run the unit test and check that it passes**

Run: `pnpm vitest run src/lib/proPrice.test.ts`
Expected: PASS (4 tests).

- [ ] **Step 5: Write the failing button spec**

```ts
// web/src/lib/components/ProCheckoutButton.spec.ts
import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import ProCheckoutButton from './ProCheckoutButton.svelte';
import type { PublicPrice } from '#lib/types.js';

const { promptSignIn, billingCheckout, authed } = vi.hoisted(() => ({
  promptSignIn: vi.fn(),
  billingCheckout: vi.fn(),
  authed: { value: false },
}));
vi.mock('$app/paths', () => ({ resolve: (p: string) => p }));
vi.mock('#lib/signin.js', () => ({ promptSignIn }));
vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: () => authed.value }));
vi.mock('#lib/api.js', () => ({ api: { billingCheckout } }));

const price = { id: 'price_m', amount_cents: 500, currency: 'usd', interval: 'month', default: true, tier: 'pro' } as PublicPrice;

beforeEach(() => {
  promptSignIn.mockReset();
  billingCheckout.mockReset();
  authed.value = false;
});

describe('ProCheckoutButton', () => {
  it('links to /pricing when no price is known', () => {
    render(ProCheckoutButton, { price: null });
    expect(screen.getByRole('link', { name: 'See Pro plans' }).getAttribute('href')).toBe('/pricing');
  });

  it('quotes the price and asks a signed-out visitor to sign in', async () => {
    render(ProCheckoutButton, { price });
    await fireEvent.click(screen.getByRole('button', { name: 'Get Pro — $5/mo' }));
    expect(promptSignIn).toHaveBeenCalledOnce();
    expect(billingCheckout).not.toHaveBeenCalled();
  });

  it('opens checkout for a signed-in visitor', async () => {
    authed.value = true;
    billingCheckout.mockResolvedValue({ url: 'https://checkout.example/s' });
    render(ProCheckoutButton, { price });
    await fireEvent.click(screen.getByRole('button', { name: 'Get Pro — $5/mo' }));
    expect(billingCheckout).toHaveBeenCalledWith('price_m');
  });

  it('says so when checkout fails', async () => {
    authed.value = true;
    billingCheckout.mockRejectedValue(new Error('down'));
    render(ProCheckoutButton, { price });
    await fireEvent.click(screen.getByRole('button', { name: 'Get Pro — $5/mo' }));
    expect(await screen.findByText(/Checkout is not available/)).toBeTruthy();
  });
});
```

- [ ] **Step 6: Run it and check that it fails**

Run: `pnpm vitest run src/lib/components/ProCheckoutButton.spec.ts`
Expected: FAIL, because the component does not exist.

- [ ] **Step 7: Implement the button**

```svelte
<!-- web/src/lib/components/ProCheckoutButton.svelte -->
<!-- The Pro call to action on a feature landing: straight to checkout at the monthly
     price, the same session /pricing opens. A promo code is entered on /pricing only; a
     discount the account already holds applies here too, since checkout reads it. With
     no price known it falls back to the plan page rather than a dead button. -->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { priceLabel } from '#lib/proPrice.js';
  import { promptSignIn } from '#lib/signin.js';
  import type { PublicPrice } from '#lib/types.js';
  import { Button } from '#lib/ui/index.js';

  const { price, size = 'lg' }: { price: PublicPrice | null; size?: 'lg' | 'default' } = $props();

  let busy = $state(false);
  let error = $state<string | null>(null);

  async function checkout() {
    if (!price || busy) return;
    if (!isAuthenticated()) {
      promptSignIn();
      return;
    }
    busy = true;
    error = null;
    try {
      const { url } = await api.billingCheckout(price.id);
      window.location.href = url;
    } catch {
      error = 'Checkout is not available right now. Please try again in a moment.';
      busy = false;
    }
  }
</script>

{#if price}
  <Button variant="primary" {size} disabled={busy} onclick={checkout}>
    Get Pro — {priceLabel(price)}/mo
  </Button>
  {#if error}<p class="w-full text-sm text-destructive" role="alert">{error}</p>{/if}
{:else}
  <Button href={resolve('/pricing')} variant="primary" {size}>See Pro plans</Button>
{/if}
```

- [ ] **Step 8: Run the spec and check that it passes**

Run: `pnpm vitest run src/lib/components/ProCheckoutButton.spec.ts src/lib/proPrice.test.ts`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add src/lib/proPrice.ts src/lib/proPrice.test.ts src/lib/proPerks.ts src/lib/components/ProCheckoutButton.svelte src/lib/components/ProCheckoutButton.spec.ts src/routes/pricing/+page.svelte
git commit -m "Add a Pro checkout button and share the Pro perk list"
```

---

### Task 4: The FAQ, rewritten around buyer objections

**Files:**
- Modify: `web/src/lib/tailorFaq.ts`, which is replaced whole.
- Test: `web/src/lib/tailorFaq.test.ts` stays unchanged and must still pass.

- [ ] **Step 1: Replace the array**

```ts
// CV-tailoring landing FAQ — the single source for both the visible section
// (TailorLandingView.svelte) and the FAQPage JSON-LD (routes/features/tailor).
// Google requires the schema's answers to match the on-page text, so they share
// this. Written around what stops a purchase, answer-first, and honest to what the
// product does (internal/candidate/cvedit, internal/candidate/experience).

import type { FaqItem } from './seo';

export const TAILOR_FAQ: FaqItem[] = [
  {
    question: 'Will a recruiter be able to tell an AI wrote it?',
    answer:
      'The words are yours to begin with. The agent reorders and rephrases what you already did, in the vacancy\'s own terms, one edit at a time — and you can see, change or undo each of them. There is no wholesale rewrite in a generic voice to give it away.',
  },
  {
    question: 'Can it put things on my CV that aren\'t true?',
    answer:
      'No. Before it writes anything, it splits the job\'s requirements into what your history covers and what it does not. The first it brings forward; the second it asks you about. Anything a model merely inferred is stored as the model\'s and cannot go into a CV until you confirm it — that check is in the code, not in a prompt.',
  },
  {
    question: 'What happens to my original CV?',
    answer:
      'Nothing. Tailoring copies it into a new CV bound to one vacancy, so every application gets its own version and your base CV is never edited.',
  },
  {
    question: 'Will an ATS read the PDF?',
    answer:
      'Yes. The CV is typeset into a single-column PDF with real text — no tables, no side columns, no words inside an image — so the parser reads what you see. Templates change the typography, never the content.',
  },
  {
    question: 'How much does it cost, and how do I cancel?',
    answer:
      'Tailoring is part of Pro, which also covers cover letters, the AI assistant and auto-apply. It is billed monthly and you can cancel any time from your plan page; Pro stays on until the end of the period you paid for.',
  },
  {
    question: 'What happens to my data?',
    answer:
      'Your CV is used to tailor your applications and nothing else. Contact details are masked before the CV text reaches a model, and the whole codebase is open source, so you can check how it is handled.',
  },
  {
    question: 'Can I drive it from my own agent or the terminal?',
    answer:
      'Yes. The freehire CLI runs the same loop with one API key — read the analysis, read the CV, apply one edit, render the PDF — under the same rules, so your agent cannot write in what you have not done either.',
  },
];
```

Before you commit, check two claims against the code:
1. Contact masking: `internal/candidate/pii/AGENTS.md` must say that CV text is masked before it reaches the LLM on the tailoring path. If it does not, delete the sentence "Contact details are masked…".
2. "Pro stays on until the end of the period you paid for": `internal/identity/billing/AGENTS.md` must describe cancellation at period end. If it does not, delete that clause.

- [ ] **Step 2: Run the FAQ test**

Run: `pnpm vitest run src/lib/tailorFaq.test.ts`
Expected: PASS (4 tests).

- [ ] **Step 3: Commit**

```bash
git add src/lib/tailorFaq.ts
git commit -m "Rewrite the tailoring FAQ around what stops a purchase"
```

---

### Task 5: The page load and the rewritten landing

**Files:**
- Create: `web/src/routes/features/tailor/+page.server.ts`
- Modify: `web/src/routes/features/tailor/+page.svelte`
- Modify: `web/src/lib/components/TailorLandingView.svelte`, which is replaced whole.

**Interfaces:**
- Consumes: `proMonthlyPrice`, `priceLabel`, `PRO_PERKS`, `ProCheckoutButton`, `TailorHeroDemo`, `DEMO_*`, `highlight`, `TAILOR_FAQ`.
- Produces: `TailorLandingView` takes the prop `{ price: PublicPrice | null }`.

- [ ] **Step 1: Server load**

```ts
// web/src/routes/features/tailor/+page.server.ts
import { serverApi } from '#lib/server/api.js';
import { proMonthlyPrice } from '#lib/proPrice.js';
import type { PageServerLoad } from './$types';

// The price is loaded on the server so the call to action carries it in the HTML. It
// reads no cookie: this page is cached by URL alone. A failed load drops the price, and
// the button falls back to /pricing — a missing price is better than a stale one.
export const load: PageServerLoad = async ({ fetch }) => {
  try {
    return { price: proMonthlyPrice(await serverApi(fetch).plans()) };
  } catch {
    return { price: null };
  }
};
```

- [ ] **Step 2: Page wrapper**

In `+page.svelte`:
- Add `import type { PageData } from './$types';` and `const { data }: { data: PageData } = $props();`.
- Render `<TailorLandingView price={data.price} />`.
- Replace the `<Seo>` title with `"CV tailoring — your CV rewritten for each job, nothing made up | freehire"`.
- Replace the description with `"freehire rewrites your CV for one vacancy at a time: it brings your relevant work forward in the job's own words, asks you about anything your history doesn't support, and shows the job match rising. ATS-readable PDF. Part of Pro."`

- [ ] **Step 3: Rewrite `TailorLandingView.svelte`**

```svelte
<script lang="ts">
  import { resolve } from '$app/paths';
  import ProCheckoutButton from './ProCheckoutButton.svelte';
  import TailorHeroDemo from './TailorHeroDemo.svelte';
  import { NumberedGrid, SectionLabel } from '#lib/ui/index.js';
  import { priceLabel } from '#lib/proPrice.js';
  import { PRO_PERKS } from '#lib/proPerks.js';
  import { DEMO_BULLETS, DEMO_ROLE, highlight } from '#lib/tailorDemo.js';
  import { TAILOR_FAQ } from '#lib/tailorFaq.js';
  import type { PublicPrice } from '#lib/types.js';

  const { price }: { price: PublicPrice | null } = $props();

  const facts = ['3.3M+ live jobs', 'ATS-readable PDF', '9 templates', 'Open source · 3.2k★'];

  const split = [
    {
      key: 'have',
      title: "You have it, it's just buried",
      body: 'Things the job asks for that your history already covers, but your CV mentions in passing or three roles down. These get brought forward and said plainly.',
      sample: ['Kafka at scale', 'Payments migrations', 'Mentoring'],
    },
    {
      key: 'gap',
      title: "You don't have it yet",
      body: 'Things nothing in your history supports. The agent asks you about them. It cannot write them in on its own.',
      sample: ['Kubernetes operators', 'Rust'],
    },
  ];

  const steps = [
    { n: '01', title: 'Pick a job', body: 'Any live posting in the catalogue. Tailoring always works toward one real vacancy, not a pasted description.' },
    { n: '02', title: 'See the match', body: 'The match analysis scores your CV against the job and names what is missing. That is what the agent works from.' },
    { n: '03', title: 'Tailor with the agent, or by hand', body: 'It proposes one edit at a time and you watch the CV change. The editor is right there, so you can overrule anything.' },
    { n: '04', title: 'Download the PDF', body: 'Pick a template and export. Your original CV stays untouched; this job gets its own copy.' },
  ];

  // Every cell must be true for every product in its column. "Varies" where a category
  // differs product to product; never a guess. No product is named.
  const priceCell = price ? `${priceLabel(price)} / month` : 'Pro plan';
  const compare = [
    { row: 'Works from a real, live job posting', us: 'Yes', chat: 'You paste it in', builder: 'You paste it in' },
    { row: "Won't add experience you don't have", us: 'Enforced in code', chat: 'Depends on your prompt', builder: 'Varies' },
    { row: 'Job match score, before and after', us: 'Yes', chat: 'No', builder: 'Varies' },
    { row: 'ATS-readable PDF', us: 'Yes', chat: 'You format it', builder: 'Varies' },
    { row: 'Every edit visible and undoable', us: 'Yes', chat: 'Rewrites the whole text', builder: 'Varies' },
    { row: 'Price', us: priceCell, chat: 'Varies', builder: 'Varies' },
  ];
</script>

<div class="flex flex-col">
  <!-- Hero -->
  <section class="dot-grid -mx-4 grid items-center gap-12 px-4 pb-16 pt-8 lg:grid-cols-[1fr_1fr]">
    <div>
      <SectionLabel text="cv tailoring · pro" />
      <h1 class="mt-6 max-w-2xl text-balance text-4xl font-semibold leading-[1.0] tracking-tighter sm:text-6xl">
        Your CV, rewritten for this job. Nothing made up.
      </h1>
      <p class="mt-7 max-w-xl text-lg leading-relaxed text-muted-foreground">
        freehire reads the vacancy, brings your relevant work forward in the job's own words, and
        asks you about anything it can't find in your history.
      </p>
      <div class="mt-9 flex flex-wrap items-center gap-3">
        <ProCheckoutButton {price} />
        <a href="#how-it-works" class="text-sm font-medium text-muted-foreground hover:text-foreground">See how it works ↓</a>
      </div>
      <p class="mt-3 text-xs text-muted-foreground">Cancel any time · Your original CV is never touched.</p>
    </div>
    <TailorHeroDemo />
  </section>

  <!-- Fact strip -->
  <ul class="grid grid-cols-2 gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-4">
    {#each facts as f (f)}
      <li class="bg-background px-4 py-3 text-center text-sm font-medium">{f}</li>
    {/each}
  </ul>

  <!-- Before and after -->
  <section class="py-16 sm:py-20">
    <SectionLabel text="before → after" />
    <h2 class="mt-6 max-w-2xl text-3xl font-semibold tracking-tight sm:text-4xl">
      Same facts. Said for this job.
    </h2>
    <p class="mt-3 text-sm text-muted-foreground">
      Example, tailored for <span class="font-medium text-foreground">{DEMO_ROLE}</span>. Highlighted words come from the vacancy.
    </p>
    <ol class="mt-8 grid gap-4">
      {#each DEMO_BULLETS as b (b.req)}
        <li class="grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-2">
          <p class="bg-background p-4 text-sm text-muted-foreground line-through decoration-muted-foreground/40">{b.before}</p>
          <p class="bg-background p-4 text-sm">
            {#each highlight(b.after, b.keywords) as part, i (i)}
              {#if part.hit}<mark class="rounded bg-brand/15 px-0.5 text-foreground">{part.text}</mark>{:else}{part.text}{/if}
            {/each}
          </p>
        </li>
      {/each}
    </ol>
  </section>

  <!-- Honesty split -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="it doesn't make things up" />
    <h2 class="mt-6 max-w-2xl text-3xl font-semibold tracking-tight sm:text-4xl">
      It knows the difference between burying something and not having it.
    </h2>
    <div class="mt-10 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-2">
      {#each split as s (s.key)}
        <div class="bg-background p-6 sm:p-7">
          <h3 class="text-lg font-semibold tracking-tight">{s.title}</h3>
          <p class="mt-2 text-sm leading-relaxed text-muted-foreground">{s.body}</p>
          <div class="mt-4 flex flex-wrap gap-2">
            {#each s.sample as item (item)}
              <span class="rounded-md border px-2 py-0.5 font-mono text-xs {s.key === 'have' ? 'border-success-strong/50 text-success-strong' : 'border-warning/50 text-warning-strong'}">{item}</span>
            {/each}
          </div>
        </div>
      {/each}
    </div>
    <p class="mt-6 max-w-3xl text-sm text-muted-foreground">
      This rule lives in the code, not in the agent's instructions, and the code is open source.
    </p>
  </section>

  <!-- How it works -->
  <section id="how-it-works" class="scroll-mt-20 border-t border-border py-16 sm:py-20">
    <SectionLabel text="how it works" />
    <NumberedGrid items={steps} class="mt-10 sm:grid-cols-2" />
  </section>

  <!-- What Pro gets you -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="pro" />
    <div class="mt-6 grid gap-10 lg:grid-cols-2 lg:items-center">
      <div>
        <h2 class="max-w-md text-3xl font-semibold tracking-tight sm:text-4xl">
          Tailoring is part of Pro{price ? ` — ${priceLabel(price)} a month` : ''}.
        </h2>
        <p class="mt-4 max-w-md text-muted-foreground">Cancel any time. Search, alerts and tracking stay free.</p>
        <div class="mt-8 flex flex-wrap items-center gap-3">
          <ProCheckoutButton {price} />
          <a href={resolve('/pricing')} class="text-sm font-medium text-muted-foreground hover:text-foreground">Compare plans</a>
        </div>
      </div>
      <ul class="grid gap-px overflow-hidden rounded-xl border border-border bg-border">
        {#each PRO_PERKS as perk (perk.text)}
          <li class="bg-background">
            <a href={perk.href} class="block px-5 py-4 text-sm font-medium hover:bg-secondary/50">{perk.text}</a>
          </li>
        {/each}
      </ul>
    </div>
  </section>

  <!-- Comparison -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="compared" />
    <h2 class="mt-6 max-w-2xl text-3xl font-semibold tracking-tight sm:text-4xl">Why not just ask a chatbot?</h2>
    <div class="mt-10 overflow-x-auto rounded-xl border border-border">
      <table class="w-full min-w-[36rem] text-left text-sm">
        <thead class="bg-secondary/50 text-xs uppercase tracking-wide text-muted-foreground">
          <tr>
            <th scope="col" class="px-4 py-3 font-medium"><span class="sr-only">Feature</span></th>
            <th scope="col" class="px-4 py-3 font-semibold text-foreground">freehire</th>
            <th scope="col" class="px-4 py-3 font-medium">Pasting into a chatbot</th>
            <th scope="col" class="px-4 py-3 font-medium">A CV builder</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-border">
          {#each compare as c (c.row)}
            <tr>
              <th scope="row" class="px-4 py-3 font-medium">{c.row}</th>
              <td class="px-4 py-3 font-medium text-success-strong">{c.us}</td>
              <td class="px-4 py-3 text-muted-foreground">{c.chat}</td>
              <td class="px-4 py-3 text-muted-foreground">{c.builder}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  </section>

  <!-- Founder note -->
  <section class="border-t border-border py-16 sm:py-20">
    <figure class="mx-auto max-w-2xl">
      <blockquote class="text-xl leading-relaxed tracking-tight sm:text-2xl">
        It is easy for an AI to write that you led a Kubernetes migration you never touched.
        It reads well right up to the first interview question about it. So the agent here works the
        other way round: it only uses what you actually did, and when the job asks for something it
        can't find in your history, it asks you instead of writing it. That rule lives in the code,
        not in a prompt, and the code is open for you to check.
      </blockquote>
      <figcaption class="mt-6 text-sm text-muted-foreground">— Ilya Strelov, founder</figcaption>
    </figure>
  </section>

  <!-- FAQ. Visible answers and the FAQPage JSON-LD share TAILOR_FAQ. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="faq" />
    <h2 class="mt-6 max-w-md text-3xl font-semibold tracking-tight sm:text-4xl">Questions people ask before buying.</h2>
    <dl class="mt-10 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-2">
      {#each TAILOR_FAQ as item (item.question)}
        <div class="bg-background p-6 sm:p-7">
          <dt class="text-lg font-semibold tracking-tight">{item.question}</dt>
          <dd class="mt-2 text-sm leading-relaxed text-muted-foreground">{item.answer}</dd>
        </div>
      {/each}
    </dl>
  </section>

  <!-- Closing CTA -->
  <section class="border-t border-border py-16 sm:py-20">
    <div class="flex flex-col items-start gap-4 rounded-xl border border-border bg-secondary/40 p-6 sm:p-8">
      <h2 class="text-2xl font-semibold tracking-tight">Stop sending the same CV everywhere.</h2>
      <p class="max-w-xl text-muted-foreground">One job, one tailored CV, and nothing on it you didn't do.</p>
      <div class="flex flex-wrap items-center gap-3">
        <ProCheckoutButton {price} />
        <a href={resolve('/jobs')} class="text-sm font-medium text-muted-foreground hover:text-foreground">Browse jobs first</a>
      </div>
    </div>
  </section>
</div>
```

Notes for the implementer:
- `priceCell` reads `price` outside `$derived`. Svelte warns that this captures only the initial value. Write it as `const priceCell = $derived(price ? ... : 'Pro plan');` and build `compare` with `$derived` too.
- `#how-it-works` is a same-page anchor. If the `svelte/no-navigation-without-resolve` lint rule flags it, use `href={resolve('/features/tailor') + '#how-it-works'}`, the same pattern other pages use.
- Check whether `TailorWorkspacePreview` still has importers, using `grep -rn TailorWorkspacePreview src`. It must still be used by the onboarding tour. If the landing page was its only importer, stop and report it instead of deleting the file.

- [ ] **Step 4: Type and lint check**

Run: `pnpm check && pnpm lint`
Expected: no new errors. Fix any reported in the touched files.

- [ ] **Step 5: Run all web tests**

Run: `pnpm vitest run`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add src/routes/features/tailor src/lib/components/TailorLandingView.svelte
git commit -m "Rebuild the tailoring landing as a Pro sales page"
```

---

### Task 6: Look at it in a real browser

**Files:** none, unless the check finds something to fix.

- [ ] **Step 1:** Start the dev server with `pnpm dev`, pointed at the prod API the way `web/AGENTS.md` describes for local UI work. Open `http://localhost:5173/features/tailor`.
- [ ] **Step 2:** Check the following at 1440×900 and 390×844, in light and in dark mode:
  - The hero demo loops: the score climbs 54 → 86 and the amber note appears at the end.
  - The button reads "Get Pro — $5/mo". While signed out, a click goes to `/signin`.
  - The table scrolls sideways on mobile instead of overflowing.
  - No text runs over the score badge in the demo.
- [ ] **Step 3:** In the browser devtools, emulate `prefers-reduced-motion: reduce` and reload. The demo shows the final state and does not move.
- [ ] **Step 4:** Fix anything found, run `pnpm check && pnpm vitest run`, and commit as "Polish the tailoring landing after a browser pass".
