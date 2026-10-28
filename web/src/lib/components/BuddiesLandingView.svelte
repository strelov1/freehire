<script lang="ts">
  import { resolve } from '$app/paths';
  import BuddiesHeroDemo from './BuddiesHeroDemo.svelte';
  import { Button, NumberedGrid, SectionLabel } from '#lib/ui/index.js';
  import { BUDDIES_FAQ } from '#lib/buddiesFaq.js';

  const facts = ['Free for everyone', 'Anonymous handle', 'Members only, no crawlers', 'You choose what shows'];

  const steps = [
    { n: '01', title: 'Join', body: 'One click. You get a random handle like amber-heron-42 — no name, no photo.' },
    { n: '02', title: 'Meet people like you', body: 'Members ranked by the same category, the same country and region, and the companies you both applied to in the last 90 days.' },
    { n: '03', title: 'Follow their progress', body: 'A feed of who applied where and who reached the interview or the offer, by day.' },
    { n: '04', title: "Ask the one who's been there", body: 'Invite a member to chat with a first message of up to 500 characters. Declining is silent.' },
  ];

  const visibility = [
    {
      key: 'seen',
      title: 'Others see',
      items: ['Your handle', 'Role titles, as you wrote them', 'Category, seniority, skills and country', 'Applications and interview or offer moves, with company and day'],
      chip: 'border-success-strong/50 text-success-strong',
    },
    {
      key: 'hidden',
      title: 'Nobody sees',
      items: ['Your name or email', 'Anything else in your CV', 'Rejections and withdrawals', 'The offer you accepted', 'Postings pasted privately'],
      chip: 'border-border text-muted-foreground',
    },
  ];

  const controls = [
    { title: 'Block', body: 'Ends the conversation for good and hides you from each other everywhere.' },
    { title: 'Report', body: 'Sends the conversation to our moderators.' },
    { title: 'Leave', body: 'Hides you and closes your chats. Come back any time with the same handle.' },
  ];
</script>

{#snippet join()}
  <Button href={resolve('/my/buddies')} variant="primary" size="lg">Join interview buddies</Button>
{/snippet}

<div class="flex flex-col">
  <!-- Hero -->
  <section class="dot-grid -mx-4 grid items-center gap-12 px-4 pb-16 pt-8 lg:grid-cols-2">
    <div>
      <SectionLabel text="interview buddies · free" />
      <h1 class="mt-6 max-w-2xl text-balance text-4xl font-semibold leading-none tracking-tighter sm:text-6xl">
        Find people interviewing where you are.
      </h1>
      <p class="mt-7 max-w-xl text-lg leading-relaxed text-muted-foreground">
        Join anonymously, see who's applying to the same companies and how far they got, and
        message the one who just had the interview you're preparing for.
      </p>
      <div class="mt-9 flex flex-wrap items-center gap-3">
        {@render join()}
        <a href="#how-it-works" class="text-sm font-medium text-muted-foreground hover:text-foreground">See how it works ↓</a>
      </div>
      <p class="mt-3 text-xs text-muted-foreground">Free · Anonymous · Leave any time</p>
    </div>
    <BuddiesHeroDemo />
  </section>

  <!-- Fact strip -->
  <ul class="grid grid-cols-2 gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-4">
    {#each facts as f (f)}
      <li class="bg-background px-4 py-3 text-center text-sm font-medium">{f}</li>
    {/each}
  </ul>

  <!-- How it works -->
  <section id="how-it-works" class="scroll-mt-20 py-16 sm:py-20">
    <SectionLabel text="how it works" />
    <NumberedGrid items={steps} class="mt-10 sm:grid-cols-2" />
  </section>

  <!-- Visibility -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="what others see" />
    <h2 class="mt-6 max-w-2xl text-3xl font-semibold tracking-tight sm:text-4xl">
      Your progress is visible. You aren't.
    </h2>
    <div class="mt-10 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-2">
      {#each visibility as v (v.key)}
        <div class="bg-background p-6 sm:p-7">
          <h3 class="text-lg font-semibold tracking-tight">{v.title}</h3>
          <ul class="mt-4 flex flex-wrap gap-2">
            {#each v.items as item (item)}
              <li class="rounded-md border px-2 py-0.5 font-mono text-xs {v.chip}">{item}</li>
            {/each}
          </ul>
        </div>
      {/each}
    </div>
    <p class="mt-6 max-w-3xl text-sm text-muted-foreground">
      Hide any single application from the feed with one switch on the job in your tracker.
    </p>
  </section>

  <!-- Control -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="you stay in control" />
    <h2 class="mt-6 max-w-2xl text-3xl font-semibold tracking-tight sm:text-4xl">Leave, block or report at any time.</h2>
    <div class="mt-10 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-3">
      {#each controls as c (c.title)}
        <div class="bg-background p-6 sm:p-7">
          <h3 class="text-lg font-semibold tracking-tight">{c.title}</h3>
          <p class="mt-2 text-sm leading-relaxed text-muted-foreground">{c.body}</p>
        </div>
      {/each}
    </div>
  </section>

  <!-- FAQ. Visible answers and the FAQPage JSON-LD share BUDDIES_FAQ. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="faq" />
    <h2 class="mt-6 max-w-md text-3xl font-semibold tracking-tight sm:text-4xl">Questions people ask before joining.</h2>
    <dl class="mt-10 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-2">
      {#each BUDDIES_FAQ as item (item.question)}
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
      <h2 class="text-2xl font-semibold tracking-tight">Stop interviewing alone.</h2>
      <p class="max-w-xl text-muted-foreground">Free, anonymous, and you can leave any time.</p>
      {@render join()}
    </div>
  </section>
</div>
