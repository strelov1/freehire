<script lang="ts">
  import { resolve } from '$app/paths';
  import { ArrowRight, ShieldCheck, BadgeCheck, Timer, EyeOff, Handshake, Send } from '@lucide/svelte';
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
      icon: EyeOff,
      title: 'Referrers stay anonymous',
      body: 'A referrer never sees who you are until they choose to contact you, and you never see them. The intro happens on neutral ground.',
    },
    {
      icon: BadgeCheck,
      title: 'Real employees only',
      body: 'Every offer to refer is backed by proof of employment that a moderator reviews before the company becomes referral-eligible.',
    },
    {
      icon: Timer,
      title: 'No spraying',
      body: 'A rolling daily cap keeps requests deliberate — this is a warm introduction, not a mass-apply button.',
    },
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

<div class="flex flex-col gap-20 sm:gap-28">
  <!-- ── Hero ─────────────────────────────────────────────────────────────── -->
  <section class="reveal-hero relative isolate flex flex-col gap-7 pt-4">
    <!-- soft brand glow, contained; decorative only -->
    <div class="glow pointer-events-none absolute -left-24 -top-24 -z-10 h-80 w-80 rounded-full" aria-hidden="true"></div>

    <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">
      // referrals · a warm way in
    </p>

    <h1 class="max-w-3xl text-balance text-4xl font-semibold leading-[0.98] tracking-tighter sm:text-6xl">
      Referred candidates get seen.
      <span class="mt-2 block text-muted-foreground sm:ml-[1.5ch]">Everyone else waits in the pile.</span>
    </h1>

    <p class="max-w-xl text-lg leading-relaxed text-muted-foreground">
      freehire quietly connects you with someone already inside the company — an employee who can put
      your name forward. They stay anonymous, you skip the cold apply, and the intro comes warm.
    </p>

    <!-- signature glyph: the path in one line -->
    <p class="flex flex-wrap items-center gap-2 font-mono text-xs text-muted-foreground">
      <span class="rounded-md bg-secondary/60 px-2 py-1 text-foreground">you</span>
      <ArrowRight class="size-3.5" />
      <span class="rounded-md border border-brand/25 bg-brand-muted px-2 py-1 text-brand-strong">insider</span>
      <ArrowRight class="size-3.5" />
      <span class="rounded-md bg-secondary/60 px-2 py-1 text-foreground">interview</span>
    </p>

    <div class="flex flex-wrap items-center gap-3">
      <Button href={askCta} variant="primary" size="lg">Ask for a referral</Button>
      <Button href={referrerCta} variant="outline" size="lg">Refer someone in</Button>
    </div>

    <p class="font-mono text-xs text-muted-foreground">
      free · anonymous referrers · employment verified
    </p>
  </section>

  <!-- ── The warm path ────────────────────────────────────────────────────── -->
  <section class="flex flex-col gap-8">
    <div class="flex flex-col gap-2">
      <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// how it happens</p>
      <h2 class="max-w-xl text-2xl font-semibold tracking-tight">Three beats from cold list to warm intro</h2>
    </div>

    <ol class="flex flex-col gap-4 md:flex-row md:items-stretch md:gap-0">
      {#each steps as step, i (step.n)}
        <li class="path-node flex flex-1 flex-col gap-3 rounded-xl border border-border p-6">
          <span class="font-mono text-sm text-brand-strong">{step.n}</span>
          <h3 class="text-base font-semibold tracking-tight">{step.title}</h3>
          <p class="text-sm leading-relaxed text-muted-foreground">{step.body}</p>
        </li>
        {#if i < steps.length - 1}
          <div class="hidden shrink-0 items-center justify-center px-3 md:flex" aria-hidden="true">
            <ArrowRight class="size-5 text-muted-foreground/60" />
          </div>
        {/if}
      {/each}
    </ol>
  </section>

  <!-- ── The one line that matters ────────────────────────────────────────── -->
  <section class="reveal-stat relative overflow-hidden rounded-xl border border-brand/25 bg-brand-muted px-6 py-12 sm:px-12 sm:py-16">
    <p class="font-mono text-xs uppercase tracking-[0.2em] text-brand-strong/70">// the whole pitch</p>
    <p class="mt-4 max-w-3xl text-balance text-3xl font-semibold leading-tight tracking-tight text-brand-strong sm:text-5xl">
      One warm intro beats a hundred cold applications.
    </p>
    <p class="mt-5 flex items-center gap-3 font-mono text-sm text-brand-strong/80">
      <Handshake class="size-5" />
      referrals are how most roles are actually filled — this is your side door in.
    </p>
  </section>

  <!-- ── Two sides ────────────────────────────────────────────────────────── -->
  <section class="flex flex-col gap-8">
    <div class="flex flex-col gap-2">
      <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// pick your side</p>
      <h2 class="max-w-xl text-2xl font-semibold tracking-tight">Two ways to use it</h2>
    </div>

    <div class="grid gap-6 lg:grid-cols-2">
      <!-- Seeker -->
      <div class="flex flex-col gap-5 rounded-xl border border-border p-7">
        <div class="flex items-center gap-3">
          <span class="flex size-9 items-center justify-center rounded-lg bg-secondary/70">
            <Send class="size-4.5 text-foreground" />
          </span>
          <h3 class="text-lg font-semibold tracking-tight">Looking for a way in</h3>
        </div>
        <p class="text-sm leading-relaxed text-muted-foreground">
          Find a company that has a referrer, attach your CV, and ask. If someone takes it up, they
          reach out to you directly — no application black hole.
        </p>
        <ul class="flex flex-col gap-2.5 text-sm">
          {#each ['Browse companies and open one with a referrer', 'Attach your CV — uploaded or tailored', 'Leave a contact and a short note', 'Wait for a warm reply'] as point (point)}
            <li class="flex items-start gap-2.5 leading-relaxed">
              <ArrowRight class="mt-0.5 size-4 shrink-0 text-brand-strong" />
              <span class="text-muted-foreground">{point}</span>
            </li>
          {/each}
        </ul>
        <div class="mt-auto pt-1">
          <Button href={browseCta} variant="primary" size="md">Find a company to ask</Button>
        </div>
      </div>

      <!-- Referrer — brand-tinted to set it apart -->
      <div class="flex flex-col gap-5 rounded-xl border border-brand/25 bg-brand-muted/50 p-7">
        <div class="flex items-center gap-3">
          <span class="flex size-9 items-center justify-center rounded-lg bg-brand-muted">
            <Handshake class="size-4.5 text-brand-strong" />
          </span>
          <h3 class="text-lg font-semibold tracking-tight text-brand-strong">Already inside</h3>
        </div>
        <p class="text-sm leading-relaxed text-brand-strong/80">
          Refer good people into your company without exposing yourself. Offer once, get vetted
          requests, and act only on the ones you like.
        </p>
        <ul class="flex flex-col gap-2.5 text-sm">
          {#each ['Offer to refer for your company', 'Upload proof of employment once — reviewed by a moderator', 'Receive matching requests, stay anonymous', 'Reach out to the ones worth it'] as point (point)}
            <li class="flex items-start gap-2.5 leading-relaxed">
              <ArrowRight class="mt-0.5 size-4 shrink-0 text-brand-strong" />
              <span class="text-brand-strong/90">{point}</span>
            </li>
          {/each}
        </ul>
        <div class="mt-auto pt-1">
          <Button href={referrerCta} variant="primary" size="md">Become a referrer</Button>
        </div>
      </div>
    </div>
  </section>

  <!-- ── Trust ────────────────────────────────────────────────────────────── -->
  <section class="flex flex-col gap-8">
    <div class="flex flex-col gap-2">
      <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// kept clean</p>
      <h2 class="max-w-xl text-2xl font-semibold tracking-tight">Why it stays worth trusting</h2>
    </div>
    <div class="grid gap-6 sm:grid-cols-3">
      {#each trust as t (t.title)}
        <div class="flex flex-col gap-3 rounded-xl border border-border p-6">
          <t.icon class="size-5 text-brand-strong" />
          <h3 class="text-base font-semibold tracking-tight">{t.title}</h3>
          <p class="text-sm leading-relaxed text-muted-foreground">{t.body}</p>
        </div>
      {/each}
    </div>
  </section>

  <!-- ── FAQ ──────────────────────────────────────────────────────────────── -->
  <section class="flex flex-col gap-8">
    <div class="flex flex-col gap-2">
      <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">// good to know</p>
      <h2 class="max-w-xl text-2xl font-semibold tracking-tight">Questions people ask first</h2>
    </div>
    <div class="grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-2">
      {#each faqs as f (f.q)}
        <details class="group bg-background p-6">
          <summary class="flex cursor-pointer list-none items-center justify-between gap-3 text-sm font-semibold tracking-tight">
            {f.q}
            <ArrowRight class="size-4 shrink-0 text-muted-foreground transition-transform group-open:rotate-90" />
          </summary>
          <p class="mt-3 text-sm leading-relaxed text-muted-foreground">{f.a}</p>
        </details>
      {/each}
    </div>
  </section>

  <!-- ── Closing CTA ──────────────────────────────────────────────────────── -->
  <section class="flex flex-col items-start gap-5 rounded-xl border border-border bg-secondary/40 p-8">
    <div class="flex items-center gap-3">
      <ShieldCheck class="size-6 text-brand-strong" />
      <h2 class="text-xl font-semibold tracking-tight">Skip the pile</h2>
    </div>
    <p class="max-w-xl text-sm leading-relaxed text-muted-foreground">
      Ask an insider to put your name forward, or open the door for someone else. Either way it takes
      a couple of minutes and stays anonymous.
    </p>
    <div class="flex flex-wrap gap-3">
      <Button href={askCta} variant="primary" size="lg">Ask for a referral</Button>
      <Button href={referrerCta} variant="outline" size="lg">Refer someone in</Button>
    </div>
  </section>
</div>

<style>
  /* Motion: one orchestrated page-load reveal. Staggered so the eye lands on the
     headline first, then the supporting rows. Purely decorative — gated behind
     prefers-reduced-motion so it never fights assistive settings. */
  @keyframes fade-up {
    from {
      opacity: 0;
      transform: translateY(12px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  .reveal-hero > * {
    animation: fade-up 0.55s cubic-bezier(0.22, 1, 0.36, 1) both;
  }
  .reveal-hero > *:nth-child(2) {
    animation-delay: 0.04s;
  }
  .reveal-hero > *:nth-child(3) {
    animation-delay: 0.1s;
  }
  .reveal-hero > *:nth-child(4) {
    animation-delay: 0.16s;
  }
  .reveal-hero > *:nth-child(5) {
    animation-delay: 0.22s;
  }
  .reveal-hero > *:nth-child(6) {
    animation-delay: 0.28s;
  }

  /* Soft, contained brand glow behind the hero — atmosphere without a new fill. */
  .glow {
    background: radial-gradient(circle at center, var(--brand-ring) 0%, transparent 68%);
    opacity: 0.16;
    filter: blur(8px);
  }

  .path-node {
    animation: fade-up 0.5s cubic-bezier(0.22, 1, 0.36, 1) both;
  }
  .path-node:nth-child(3) {
    animation-delay: 0.06s;
  }
  .path-node:nth-child(5) {
    animation-delay: 0.12s;
  }

  @media (prefers-reduced-motion: reduce) {
    .reveal-hero > *,
    .path-node {
      animation: none;
    }
  }
</style>
