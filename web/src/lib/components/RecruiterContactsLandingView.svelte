<script lang="ts">
  import { resolve } from '$app/paths';
  import ProCheckoutButton from './ProCheckoutButton.svelte';
  import { NumberedGrid, SectionLabel } from '#lib/ui/index.js';
  import { priceLabel } from '#lib/proPrice.js';
  import { PRO_PERKS } from '#lib/proPerks.js';
  import { RECRUITER_CONTACTS_FAQ } from '#lib/recruiterContactsFaq.js';
  import type { PublicPrice } from '#lib/types.js';

  const { price }: { price: PublicPrice | null } = $props();

  const self = resolve('/features/recruiter-contacts');

  // Illustrative only: invented people at an invented company.
  const sample = [
    { name: 'Maya Chen', title: 'Technical Recruiter', email: 'maya@acme.dev', badge: 'Verified' },
    { name: 'Tom Okafor', title: 'Talent Partner, Engineering', email: 'tom@acme.dev', badge: 'Likely' },
  ];

  const steps = [
    {
      n: '01',
      title: 'Open a job',
      body: 'Any live posting whose company has a website on record shows a "Who recruits here" block.',
    },
    {
      n: '02',
      title: 'Show the recruiters',
      body: 'Up to three people whose title says they recruit, with their work email and how sure we are it reaches them.',
    },
    {
      n: '03',
      title: 'Write to one person',
      body: 'Copy the address, or draft a short email from your CV and the vacancy, edit it, and send it from your own mail.',
    },
  ];
</script>

<div class="flex flex-col">
  <!-- Hero -->
  <section class="dot-grid -mx-4 grid items-center gap-12 px-4 pb-16 pt-8 lg:grid-cols-2">
    <div>
      <SectionLabel text="Recruiter contacts · Pro" />
      <h1 class="mt-6 max-w-2xl text-balance text-4xl font-semibold leading-none tracking-tighter sm:text-6xl">
        Skip the queue. Write to the person who hires.
      </h1>
      <p class="mt-7 max-w-xl text-lg leading-relaxed text-muted-foreground text-pretty">
        On a job page, see who recruits at that company and their work email, then send a short note of your own next to
        your application.
      </p>
      <div class="mt-9 flex flex-wrap items-center gap-3">
        <ProCheckoutButton {price} />
        <a href="#how-it-works" class="text-sm font-medium text-muted-foreground hover:text-foreground"
          >See how it works ↓</a
        >
      </div>
      <p class="mt-3 text-xs text-muted-foreground">
        A lookup that finds nobody is not counted · Cancel any time · 14-day refund
      </p>
    </div>

    <!-- What the block on a job page looks like, with made-up people. -->
    <div class="flex flex-col gap-3 rounded-xl border border-border bg-background p-5 shadow-sm" aria-hidden="true">
      <p class="text-sm font-semibold">Who recruits here</p>
      <ul class="flex flex-col divide-y divide-border">
        {#each sample as c (c.email)}
          <li class="py-2">
            <p class="text-sm font-medium">
              {c.name}
              <span
                class="ml-1 rounded-sm px-1.5 py-0.5 text-xs font-normal {c.badge === 'Verified'
                  ? 'bg-brand-muted text-brand-strong'
                  : 'bg-muted text-muted-foreground'}">{c.badge}</span
              >
            </p>
            <p class="text-xs text-muted-foreground">{c.title}</p>
            <p class="text-sm text-brand-strong">{c.email}</p>
          </li>
        {/each}
      </ul>
      <p class="text-xs text-muted-foreground">Example. Found from public sources.</p>
    </div>
  </section>

  <!-- How it works -->
  <section id="how-it-works" class="scroll-mt-20 border-t border-border py-16 sm:py-20">
    <SectionLabel text="How it works" />
    <h2 class="mt-6 max-w-2xl text-balance text-3xl font-semibold tracking-tight sm:text-4xl">
      From a posting to a person.
    </h2>
    <NumberedGrid items={steps} class="mt-10 sm:grid-cols-3" />
  </section>

  <!-- What Pro gets you -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="Pro" />
    <div class="mt-6 grid gap-10 lg:grid-cols-2 lg:items-center">
      <div>
        <h2 class="text-balance max-w-md text-3xl font-semibold tracking-tight sm:text-4xl">
          Recruiter contacts are part of Pro{price ? ` — ${priceLabel(price)} a month` : ''}.
        </h2>
        <p class="mt-4 max-w-md text-muted-foreground text-pretty">
          Cancel any time. Search, alerts and tracking stay free.
        </p>
        <div class="mt-8 flex flex-wrap items-center gap-3">
          <ProCheckoutButton {price} />
          <a href={resolve('/pricing')} class="text-sm font-medium text-muted-foreground hover:text-foreground"
            >Compare plans</a
          >
        </div>
      </div>
      <ul class="grid gap-px overflow-hidden rounded-xl border border-border bg-border">
        {#each PRO_PERKS as perk (perk.text)}
          <li class="bg-background">
            {#if perk.href === self}
              <span class="block px-5 py-4 text-sm font-medium">{perk.text}</span>
            {:else}
              <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- PRO_PERKS hrefs are built with resolve() in proPerks.ts -->
              <a href={perk.href} class="block px-5 py-4 text-sm font-medium hover:bg-secondary/50">{perk.text}</a>
            {/if}
          </li>
        {/each}
      </ul>
    </div>
  </section>

  <!-- FAQ. Visible answers and the FAQPage JSON-LD share RECRUITER_CONTACTS_FAQ. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="FAQ" />
    <h2 class="mt-6 text-balance max-w-2xl text-3xl font-semibold tracking-tight sm:text-4xl">
      Questions people ask first.
    </h2>
    <dl class="mt-10 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-2">
      {#each RECRUITER_CONTACTS_FAQ as item (item.question)}
        <div class="bg-background p-6 sm:odd:last:col-span-2 sm:p-7">
          <dt class="text-balance text-lg font-semibold tracking-tight">{item.question}</dt>
          <dd class="mt-2 leading-relaxed text-muted-foreground text-pretty">{item.answer}</dd>
        </div>
      {/each}
    </dl>
  </section>

  <!-- Closing CTA -->
  <section class="border-t border-border py-16 sm:py-20">
    <div class="flex flex-col items-start gap-4 rounded-xl border border-border bg-secondary/40 p-6 sm:p-8">
      <h2 class="text-balance text-2xl font-semibold tracking-tight">An application, plus a person who reads it.</h2>
      <div class="flex flex-wrap items-center gap-3">
        <ProCheckoutButton {price} />
        <a href={resolve('/jobs')} class="text-sm font-medium text-muted-foreground hover:text-foreground"
          >Browse jobs first</a
        >
      </div>
    </div>
  </section>
</div>
