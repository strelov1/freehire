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
      sample: ['Kafka', 'Payments migrations', 'Mentoring'],
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
  const priceCell = $derived(price ? `${priceLabel(price)} / month` : 'Pro plan');
  const compare = $derived([
    { row: 'Works from a real, live job posting', us: 'Yes', chat: 'You paste it in', builder: 'Usually you paste it in' },
    { row: "Won't add experience you don't have", us: 'Enforced in code', chat: 'Depends on your prompt', builder: 'Varies' },
    { row: 'Job match score, before and after', us: 'Yes', chat: 'Only if you ask', builder: 'Varies' },
    { row: 'ATS-readable PDF', us: 'Yes', chat: 'You format it', builder: 'Varies' },
    { row: 'Every edit visible and undoable', us: 'Yes', chat: 'Depends on how you ask', builder: 'Varies' },
    { row: 'Price', us: priceCell, chat: 'Varies', builder: 'Varies' },
  ]);
</script>

<div class="flex flex-col">
  <!-- Hero -->
  <section class="dot-grid -mx-4 grid items-center gap-12 px-4 pb-16 pt-8 lg:grid-cols-2">
    <div>
      <SectionLabel text="cv tailoring · pro" />
      <h1 class="mt-6 max-w-2xl text-balance text-4xl font-semibold leading-none tracking-tighter sm:text-6xl">
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
      <p class="mt-3 text-xs text-muted-foreground">Cancel any time · 14-day refund · Your original CV is never touched.</p>
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
          <div class="bg-background p-4">
            <p class="text-xs uppercase tracking-wide text-muted-foreground">Before</p>
            <p class="mt-1 text-sm text-muted-foreground">{b.before}</p>
          </div>
          <div class="bg-background p-4">
            <p class="text-xs uppercase tracking-wide text-foreground">After</p>
            <p class="mt-1 text-sm">
              {#each highlight(b.after, b.keywords) as part, i (i)}
                {#if part.hit}<mark class="rounded bg-brand/15 px-0.5 text-foreground">{part.text}</mark>{:else}{part.text}{/if}
              {/each}
            </p>
          </div>
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
    <h2 class="mt-6 max-w-2xl text-balance text-3xl font-semibold tracking-tight sm:text-4xl">From a posting to a PDF.</h2>
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
            {#if perk.href === resolve('/features/tailor')}
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

  <!-- Comparison -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="compared" />
    <h2 class="mt-6 max-w-2xl text-3xl font-semibold tracking-tight sm:text-4xl">Why not just ask a chatbot?</h2>
    <div class="mt-10 overflow-x-auto rounded-xl border border-border">
      <table class="w-full min-w-xl text-left text-sm">
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
        <div class="bg-background p-6 sm:odd:last:col-span-2 sm:p-7">
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
