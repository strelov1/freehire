<script lang="ts">
  import { resolve } from '$app/paths';
  import { Button } from '$lib/ui';

  // CTA destinations wired to the real feature (no invite-code program exists):
  //  - The headline "Ask for a referral" points at the referral hub (/my/referrals),
  //    the account cabinet that gates behind auth and holds both sides of the flow.
  //  - The seeker card's concrete step is browsing companies: a request is only
  //    created from a company page, where an approved referrer surfaces ReferralBlock.
  //  - Insiders offer to refer from that same cabinet's Offers tab.
  const askCta = resolve('/my/referrals');
  const browseCta = resolve('/companies');
  const referrerCta = `${resolve('/my/referrals')}?tab=offers`;

  // Illustrative referrer inbox for the hero — decorative, not live data. Mirrors
  // the real incoming-request card (ReferralsView) so the preview shows the actual
  // anonymized view a referrer acts on: the CV and the role, never who it is.
  const inbox = [
    { company: 'Linear', role: 'Senior Backend Engineer', time: '2h' },
    { company: 'Stripe', role: 'Staff Frontend Engineer', time: '5h' },
  ];

  // The warm path — the honest three beats of an employee referral. Copy mirrors
  // the mechanics in ReferralsView / RequestReferralModal (anonymous referrer,
  // seeker-chosen contact channel, moderated proof).
  const steps = [
    {
      n: '01',
      title: 'You ask',
      body: 'Pick the company, attach your CV or a tailored one, and leave a contact — Telegram, email, or both. That is the whole request.',
    },
    {
      n: '02',
      title: 'An insider picks it up',
      body: 'Your request lands with employees there who offered to refer. They see your CV and note — never your identity — and put your name forward internally.',
    },
    {
      n: '03',
      title: 'They reach out',
      body: 'If a referrer takes it on, they contact you directly over the channel you chose. No inbox to check here, no bot in the middle.',
    },
  ];

  // Trust rails — why the pool stays real and low-noise. These are enforced
  // server-side (moderated proof, rolling 24h per-seeker cap, anonymity).
  const trust = [
    {
      n: '01',
      title: 'Referrers are never on the hook',
      body: 'A referrer stays anonymous and only surfaces if they choose to reach out — so turning a request down is silent and awkward-free. No name on the line, no burned bridge, no guilt.',
    },
    {
      n: '02',
      title: 'Real employees only',
      body: 'Every offer to refer is backed by proof of employment that a moderator reviews before the company becomes referral-eligible.',
    },
    {
      n: '03',
      title: 'No spraying',
      body: 'A rolling daily cap keeps requests deliberate — this is a warm introduction, not a mass-apply button.',
    },
  ];

  const seekerPoints = [
    'Browse companies and open one with a referrer',
    'Attach your CV — uploaded or tailored',
    'Leave a contact and a short note',
    'Wait for a warm reply',
  ];
  const referrerPoints = [
    'Offer to refer for your company',
    'Upload proof of employment once — reviewed by a moderator',
    'Receive matching requests, stay anonymous',
    'Reach out to the ones worth it',
  ];

  const faqs = [
    {
      q: 'What does it cost?',
      a: 'Nothing. freehire is a free, open-source aggregator — referrals included. No fees, no paywall.',
    },
    {
      q: 'Will the referrer see my name?',
      a: 'No. Referrers only see the CV, note and contact you attach. Your identity is never surfaced — they reach out only if they decide to take your request forward.',
    },
    {
      q: 'How do I know a referrer actually works there?',
      a: 'Anyone offering to refer uploads proof of employment, and a moderator reviews it before the company appears as referral-available.',
    },
    {
      q: 'I work somewhere great — can I help people in?',
      a: 'Yes. Offer to refer from your account, upload proof once, and approved requests for your company start reaching you. You stay anonymous throughout.',
    },
  ];
</script>

<div class="landing">
  <!-- ── Hero ─────────────────────────────────────────────────────────────── -->
  <section class="dot-grid -mx-4 px-4 pb-4 pt-14 sm:pt-20">
    <div class="grid items-center gap-12 lg:grid-cols-[1.05fr_0.95fr]">
      <div>
        <p class="reveal font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground" style="--d:0ms">
          // referrals · a warm way in
        </p>

        <h1
          class="reveal mt-6 max-w-2xl text-balance text-5xl font-semibold leading-[0.95] tracking-tighter sm:text-7xl"
          style="--d:80ms"
        >
          Referred candidates get seen.
          <span class="mt-2 block text-muted-foreground">Everyone else waits in the pile.</span>
        </h1>

        <p class="reveal mt-7 max-w-xl text-lg leading-relaxed text-muted-foreground" style="--d:160ms">
          Everyone knows a referral is the fastest way in. The hard part is finding someone inside
          who'll make it. freehire surfaces the companies that already have a willing insider — and
          connects you to them, so you can stop cold-DMing strangers.
        </p>

        <!-- signature glyph: the path in one line, drawn in the muted mono register -->
        <p class="reveal mt-7 flex flex-wrap items-center gap-2 font-mono text-xs text-muted-foreground" style="--d:220ms">
          <span class="rounded-md border border-border bg-background px-2 py-1 text-foreground">you</span>
          <span aria-hidden="true">→</span>
          <span class="rounded-md border border-border bg-background px-2 py-1 text-foreground">insider</span>
          <span aria-hidden="true">→</span>
          <span class="rounded-md border border-border bg-background px-2 py-1 text-foreground">interview</span>
        </p>

        <div class="reveal mt-9 flex flex-wrap items-center gap-3" style="--d:300ms">
          <Button href={askCta} variant="primary" size="lg">Ask for a referral</Button>
          <Button href={referrerCta} variant="outline" size="lg">Refer someone in</Button>
        </div>

        <p class="reveal mt-6 font-mono text-xs text-muted-foreground" style="--d:360ms">
          free · anonymous referrers · employment verified
        </p>
      </div>

      <!-- Referrer inbox preview: the anonymized view an insider acts on — the CV
           and role, never who it is. Decorative, not live data. Hidden below lg. -->
      <div class="reveal hidden lg:block" style="--d:420ms">
        <figure class="overflow-hidden rounded-xl border border-border bg-card shadow-sm">
          <figcaption class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground">
            <span class="size-2.5 rounded-full bg-muted-foreground/30"></span>
            Referrals · Inbox
          </figcaption>
          <div class="flex flex-col gap-3 p-4">
            {#each inbox as r (r.company)}
              <article class="rounded-lg border border-border bg-background p-4">
                <div class="flex items-center justify-between gap-3">
                  <p class="text-sm font-medium">Referral request · {r.company}</p>
                  <span class="shrink-0 font-mono text-[11px] text-muted-foreground">{r.time}</span>
                </div>
                <div class="mt-2 flex items-center gap-2 text-xs text-muted-foreground">
                  <span
                    class="grid size-6 shrink-0 place-items-center rounded-full border border-border"
                    aria-hidden="true"
                  >
                    •
                  </span>
                  Anonymous candidate · {r.role}
                </div>
                <div class="mt-3 flex items-center gap-2">
                  <span class="rounded-md border border-border px-2 py-0.5 font-mono text-[11px] text-muted-foreground">
                    CV attached
                  </span>
                  <span class="flex-1"></span>
                  <span class="rounded-md bg-foreground px-2.5 py-1 text-[11px] font-medium text-background">Refer</span>
                  <span class="rounded-md border border-border px-2.5 py-1 text-[11px] font-medium text-muted-foreground">Pass</span>
                </div>
              </article>
            {/each}
          </div>
        </figure>
      </div>
    </div>
  </section>

  <!-- ── The warm path ────────────────────────────────────────────────────── -->
  <section class="border-t border-border py-16 sm:py-20">
    <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// how it happens</p>
    <dl class="mt-10 divide-y divide-border border-y border-border">
      {#each steps as step (step.n)}
        <div class="grid gap-2 py-6 sm:grid-cols-[auto_1fr] sm:gap-8">
          <span class="font-mono text-sm text-muted-foreground">{step.n}</span>
          <div>
            <dt class="text-lg font-semibold tracking-tight">{step.title}</dt>
            <dd class="mt-2 max-w-3xl text-sm leading-relaxed text-muted-foreground">{step.body}</dd>
          </div>
        </div>
      {/each}
    </dl>
  </section>

  <!-- ── The one line that matters ────────────────────────────────────────── -->
  <section class="border-t border-border py-16 sm:py-20">
    <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// the pitch</p>
    <p class="mt-6 max-w-3xl text-2xl font-medium leading-snug tracking-tight sm:text-3xl">
      One warm intro beats a hundred cold applications. Referrals are how most roles are actually
      filled — this is your side door in.
    </p>
  </section>

  <!-- ── Two sides ────────────────────────────────────────────────────────── -->
  <section class="border-t border-border py-16 sm:py-20">
    <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// pick your side</p>
    <div class="mt-10 grid gap-px overflow-hidden rounded-xl border border-border bg-border lg:grid-cols-2">
      <!-- Seeker -->
      <div class="flex flex-col bg-background p-7 sm:p-8">
        <p class="font-mono text-xs uppercase tracking-wide text-muted-foreground">for seekers</p>
        <h3 class="mt-4 text-xl font-semibold tracking-tight">No one on the inside? Solved.</h3>
        <p class="mt-3 text-sm leading-relaxed text-muted-foreground">
          The hardest part of a referral is finding someone willing to give it. freehire shows you
          which companies already have a referrer — attach your CV and ask, and if they see a fit they
          reach out directly. No cold DMs, no application black hole.
        </p>
        <ul class="mt-5 flex flex-col divide-y divide-border border-y border-border">
          {#each seekerPoints as point (point)}
            <li class="py-2.5 text-sm leading-relaxed text-muted-foreground">{point}</li>
          {/each}
        </ul>
        <div class="mt-7 pt-1">
          <Button href={browseCta} variant="primary" size="md">Find a company to ask</Button>
        </div>
      </div>

      <!-- Referrer -->
      <div class="flex flex-col bg-background p-7 sm:p-8">
        <p class="font-mono text-xs uppercase tracking-wide text-muted-foreground">for insiders</p>
        <h3 class="mt-4 text-xl font-semibold tracking-tight">Refer without the awkward "no"</h3>
        <p class="mt-3 text-sm leading-relaxed text-muted-foreground">
          Referring usually means putting your name on the line — and squirming when you have to turn
          someone down. Here you stay invisible until you decide someone's worth it. Pass on anyone,
          no explanation, and it's never tied back to you.
        </p>
        <ul class="mt-5 flex flex-col divide-y divide-border border-y border-border">
          {#each referrerPoints as point (point)}
            <li class="py-2.5 text-sm leading-relaxed text-muted-foreground">{point}</li>
          {/each}
        </ul>
        <div class="mt-7 pt-1">
          <Button href={referrerCta} variant="outline" size="md">Become a referrer</Button>
        </div>
      </div>
    </div>
  </section>

  <!-- ── Trust ────────────────────────────────────────────────────────────── -->
  <section class="border-t border-border py-16 sm:py-20">
    <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// kept clean</p>
    <div class="mt-10 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-3">
      {#each trust as t (t.n)}
        <div class="group bg-background p-6 transition-colors hover:bg-secondary/40 sm:p-7">
          <span class="font-mono text-sm text-muted-foreground transition-colors group-hover:text-foreground">
            {t.n}
          </span>
          <h3 class="mt-4 text-lg font-semibold tracking-tight">{t.title}</h3>
          <p class="mt-2 text-sm leading-relaxed text-muted-foreground">{t.body}</p>
        </div>
      {/each}
    </div>
  </section>

  <!-- ── FAQ ──────────────────────────────────────────────────────────────── -->
  <section class="border-t border-border py-16 sm:py-20">
    <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// faq</p>
    <dl class="mt-10 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-2">
      {#each faqs as f (f.q)}
        <div class="bg-background p-6 sm:p-7">
          <dt class="text-lg font-semibold tracking-tight">{f.q}</dt>
          <dd class="mt-2 text-sm leading-relaxed text-muted-foreground">{f.a}</dd>
        </div>
      {/each}
    </dl>
  </section>

  <!-- ── Closing CTA ──────────────────────────────────────────────────────── -->
  <section class="border-t border-border py-16 sm:py-20">
    <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// skip the pile</p>
    <h2 class="mt-6 max-w-md text-3xl font-semibold tracking-tight sm:text-4xl">
      Ask an insider to put your name forward.
    </h2>
    <p class="mt-5 max-w-xl leading-relaxed text-muted-foreground">
      Get referred, or open the door for someone else. Either way it takes a couple of minutes and
      stays anonymous.
    </p>
    <div class="mt-8 flex flex-wrap gap-3">
      <Button href={askCta} variant="primary" size="lg">Ask for a referral</Button>
      <Button href={referrerCta} variant="outline" size="lg">Refer someone in</Button>
    </div>
  </section>
</div>

<style>
  /* One orchestrated page-load: each .reveal rises in, staggered by its --d.
     Mirrors HomeView so the landing shares the site's motion language. */
  .reveal {
    opacity: 0;
    animation: rise 0.7s cubic-bezier(0.2, 0.7, 0.2, 1) forwards;
    animation-delay: var(--d, 0ms);
  }
  @keyframes rise {
    from {
      opacity: 0;
      transform: translateY(10px);
    }
    to {
      opacity: 1;
      transform: none;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .reveal {
      animation: none;
      opacity: 1;
    }
  }
</style>
