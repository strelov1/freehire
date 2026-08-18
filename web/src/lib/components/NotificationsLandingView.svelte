<script lang="ts">
  import { Bell, Check, ChevronRight, Mail, SlidersHorizontal, Smartphone } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { Button, EntityLogo, ProviderIcon, SectionLabel } from '$lib/ui';
  import BrandMark from '$lib/components/BrandMark.svelte';
  import { NOTIFICATIONS_FAQ } from '$lib/notificationsFaq';

  // Illustrative jobs for the digest mockup — decorative, not live data, all
  // named for a "Remote Go Backend" saved search so the subject line and the
  // rows agree with each other.
  const digestJobs = [
    { company: 'Grafana', title: 'Platform Engineer' },
    { company: 'Datadog', title: 'Senior Backend Engineer' },
    { company: 'Cloudflare', title: 'Staff Backend Engineer' },
  ];

  const triggers = [
    { key: 'match', text: 'A new match: instantly, or once a day at a time you pick.' },
    { key: 'saved', text: "A saved job you haven't applied to, 3 days after saving it." },
    {
      key: 'silent',
      text: 'An application gone quiet, at 21, 18, 15, 12 and 5 days of silence — sooner the closer the stage is to a decision.',
    },
    { key: 'interview', text: 'An interview, the moment it is scheduled.' },
  ];
</script>

{#snippet mailHeader()}
  <div class="flex items-center gap-2">
    <BrandMark class="size-5 text-foreground" />
    <span class="text-sm font-bold">freehire</span>
  </div>
{/snippet}

{#snippet mailFooter(note: string)}
  <p class="mt-6 border-t border-border pt-4 text-xs text-muted-foreground">
    {note}
    <a href={resolve('/my/notifications/settings')} class="underline-offset-4 hover:underline">Manage notifications</a>.
  </p>
{/snippet}

<div class="flex flex-col">
  <!-- Hero. Headline and pitch, then the real "save a search, get alerted"
       controls in miniature — the same ones on the job feed's filter sidebar. -->
  <section class="dot-grid -mx-4 grid items-center gap-12 px-4 pb-16 pt-8 lg:grid-cols-[1.05fr_0.95fr]">
    <div>
      <SectionLabel text="notifications" />
      <h1 class="mt-6 max-w-2xl text-balance text-4xl font-semibold leading-[1.0] tracking-tighter sm:text-6xl">
        You don't refresh the feed. It comes to you.
      </h1>
      <p class="mt-7 max-w-xl text-lg leading-relaxed text-muted-foreground">
        Save a search once and freehire watches it for you — instantly, or as a daily digest. The
        same settings also carry your tracking nudges, so a new match and a stalled application
        both reach you the same way: email, Telegram, or push.
      </p>
      <div class="mt-9 flex flex-wrap items-center gap-3">
        <Button href={resolve('/')} variant="primary" size="lg">Browse jobs</Button>
        <Button href={resolve('/my/notifications/settings')} variant="outline" size="lg">Notification settings</Button>
      </div>
    </div>

    <!-- The real filter-sidebar controls, in miniature: FilterSummaryShell's
         "All filters" + SaveSearchAlert's "Get new-job alerts", then the
         AlertChannels toggle it leads to. Static — no click handlers. -->
    <figure class="overflow-hidden rounded-xl border border-border bg-card shadow-sm">
      <figcaption class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground">
        <span class="size-2.5 rounded-full bg-muted-foreground/30"></span>
        Jobs · Filters
      </figcaption>
      <div class="flex flex-col gap-3 p-4 sm:p-5">
        <span
          class="flex h-11 w-full items-center justify-center gap-2 rounded-xl border border-border bg-background text-sm font-medium"
        >
          <SlidersHorizontal class="size-4" aria-hidden="true" />
          All filters
          <span
            class="inline-flex h-5 min-w-5 items-center justify-center rounded-full bg-brand px-1.5 text-[11px] font-semibold text-brand-foreground"
          >3</span>
        </span>
        <span
          class="flex h-11 w-full items-center justify-center gap-2 rounded-xl bg-secondary text-sm font-semibold"
        >
          <Bell class="size-4" aria-hidden="true" />
          Get new-job alerts
          <ChevronRight class="size-4 text-muted-foreground" aria-hidden="true" />
        </span>
        <div class="mt-2 flex flex-col gap-2 border-t border-border pt-4">
          <span class="flex items-center gap-1.5 text-sm text-muted-foreground">
            <Bell class="size-3.5 shrink-0" aria-hidden="true" />
            Notify me when a new job matches these filters
          </span>
          <div class="flex flex-wrap gap-2">
            <span
              class="inline-flex items-center gap-1.5 rounded-full border border-transparent bg-brand-muted px-3 py-1.5 text-xs font-semibold text-brand-strong"
            >
              <Check class="size-3.5" aria-hidden="true" />
              Email
            </span>
            <span
              class="inline-flex items-center gap-1.5 rounded-full border border-border bg-background px-3 py-1.5 text-xs font-semibold text-muted-foreground"
            >
              <Smartphone class="size-3.5" aria-hidden="true" />
              Push
            </span>
          </div>
        </div>
      </div>
    </figure>
  </section>

  <!-- The two triggers. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="two things worth knowing about" />
    <div class="mt-6 max-w-2xl">
      <h2 class="text-3xl font-semibold tracking-tight sm:text-4xl">A new match, or your own board gone quiet.</h2>
    </div>
    <div class="mt-10 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-2">
      <div class="bg-background p-6 sm:p-7">
        <h3 class="text-lg font-semibold tracking-tight">A job that just appeared</h3>
        <p class="mt-2 text-sm leading-relaxed text-muted-foreground">
          Save a search — stack, seniority, region, salary — and freehire matches new postings
          against it as they're added. Get told instantly, or once a day at a time you pick.
        </p>
        <a
          href={resolve('/my/notifications/searches')}
          class="mt-4 inline-block text-sm font-medium text-foreground underline-offset-4 hover:underline"
        >
          Manage your saved searches →
        </a>
      </div>
      <div class="bg-background p-6 sm:p-7">
        <h3 class="text-lg font-semibold tracking-tight">Your own board, going quiet</h3>
        <p class="mt-2 text-sm leading-relaxed text-muted-foreground">
          The same channels also carry your tracking nudges — a saved job you haven't applied to,
          an application gone silent, an interview coming up.
        </p>
        <a
          href={resolve('/features/tracking')}
          class="mt-4 inline-block text-sm font-medium text-foreground underline-offset-4 hover:underline"
        >
          How tracking works →
        </a>
      </div>
    </div>
  </section>

  <!-- What the two emails actually look like. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="what it looks like" />
    <div class="mt-6 max-w-2xl">
      <h2 class="text-3xl font-semibold tracking-tight sm:text-4xl">Two kinds of email, one settings page.</h2>
      <p class="mt-5 leading-relaxed text-muted-foreground">
        Both mirror the real template — the same card, the same brand button, the same opt-out
        line at the bottom.
      </p>
    </div>
    <div class="mt-10 grid gap-6 lg:grid-cols-2">
      <!-- Job-alert digest. -->
      <figure class="overflow-hidden rounded-xl border border-border bg-secondary/40 shadow-sm">
        <figcaption class="border-b border-border px-4 py-2.5 text-xs text-muted-foreground">
          3 new jobs for "Remote Go Backend"
        </figcaption>
        <div class="p-4 sm:p-5">
          <div class="rounded-xl border border-border bg-card p-5 shadow-sm">
            {@render mailHeader()}
            <h3 class="mt-4 text-base font-semibold tracking-tight">3 new jobs for &ldquo;Remote Go Backend&rdquo;</h3>
            <div class="mt-4 flex flex-col gap-2">
              {#each digestJobs as job (job.title)}
                <div class="flex items-center gap-3 rounded-lg border border-border p-2.5">
                  <EntityLogo name={job.company} shape="square" size="xs" />
                  <div class="min-w-0">
                    <p class="truncate text-sm font-medium">{job.title}</p>
                    <p class="truncate text-xs text-muted-foreground">{job.company}</p>
                  </div>
                </div>
              {/each}
            </div>
            <span class="mt-5 inline-block rounded-md bg-brand px-4 py-2 text-sm font-medium text-brand-foreground">
              View all — 12 more
            </span>
            {@render mailFooter("You're getting this because you set up a job alert on freehire.")}
          </div>
        </div>
      </figure>

      <!-- Tracking follow-up nudge. -->
      <figure class="overflow-hidden rounded-xl border border-border bg-secondary/40 shadow-sm">
        <figcaption class="border-b border-border px-4 py-2.5 text-xs text-muted-foreground">
          Time to follow up: Senior Backend Engineer at Linear
        </figcaption>
        <div class="p-4 sm:p-5">
          <div class="rounded-xl border border-border bg-card p-5 shadow-sm">
            {@render mailHeader()}
            <h3 class="mt-4 text-base font-semibold tracking-tight">Worth a follow-up?</h3>
            <div class="mt-4 flex items-center gap-3 rounded-lg border border-border p-2.5">
              <EntityLogo name="Linear" shape="square" size="xs" />
              <div class="min-w-0">
                <p class="truncate text-sm font-medium">Senior Backend Engineer</p>
                <p class="truncate text-xs text-muted-foreground">Linear</p>
              </div>
            </div>
            <p class="mt-4 text-sm leading-relaxed text-muted-foreground">Nothing has moved here in 12 days.</p>
            <span class="mt-5 inline-block rounded-md bg-brand px-4 py-2 text-sm font-medium text-brand-foreground">
              Open your tracking board
            </span>
            {@render mailFooter("You're getting this because you're tracking this application on freehire.")}
          </div>
        </div>
      </figure>
    </div>
  </section>

  <!-- Every trigger, on your channel. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="every trigger, on your channel" />
    <div class="mt-6 grid gap-10 lg:grid-cols-2 lg:items-start">
      <div>
        <h2 class="max-w-md text-3xl font-semibold tracking-tight sm:text-4xl">Four things trigger a notification.</h2>
        <ul class="mt-6 flex flex-col gap-2.5 text-sm leading-relaxed text-muted-foreground">
          {#each triggers as t (t.key)}
            <li class="flex gap-2.5">
              <span class="mt-2 size-1.5 shrink-0 rounded-full bg-muted-foreground/50"></span>
              {t.text}
            </li>
          {/each}
        </ul>
      </div>
      <div>
        <p class="max-w-md leading-relaxed text-muted-foreground">
          Pick the channel — any combination sends, and one you never connect is simply skipped —
          and set quiet hours; nothing arrives while they're on. A daily digest is exempt, since
          it already fires once at a time you chose.
        </p>
        <div class="mt-6 flex flex-wrap items-center gap-4 text-muted-foreground">
          <span class="flex items-center gap-1.5 text-sm"><Mail class="size-4" aria-hidden="true" /> Email</span>
          <span class="flex items-center gap-1.5 text-sm"><ProviderIcon provider="telegram" class="size-4" /> Telegram</span>
          <span class="flex items-center gap-1.5 text-sm"><Smartphone class="size-4" aria-hidden="true" /> Push</span>
        </div>
        <div class="mt-8">
          <Button href={resolve('/my/notifications/settings')} variant="primary" size="lg">Notification settings</Button>
        </div>
      </div>
    </div>
  </section>

  <!-- FAQ. Visible answers and the FAQPage JSON-LD share NOTIFICATIONS_FAQ. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="faq" />
    <h2 class="mt-6 max-w-md text-3xl font-semibold tracking-tight sm:text-4xl">Frequently asked questions.</h2>
    <!-- An odd number of items would leave the last row half empty (see the
         features doorway grid in HomeView.svelte) — the last card fills the
         row instead when the count is odd. -->
    <dl
      class="mt-10 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-2 sm:[&>*:last-child:nth-child(odd)]:col-span-2"
    >
      {#each NOTIFICATIONS_FAQ as item (item.question)}
        <div class="bg-background p-6 sm:p-7">
          <dt class="text-lg font-semibold tracking-tight">{item.question}</dt>
          <dd class="mt-2 text-sm leading-relaxed text-muted-foreground">{item.answer}</dd>
        </div>
      {/each}
    </dl>
  </section>

  <!-- Closing CTA. -->
  <section class="border-t border-border py-16 sm:py-20">
    <div class="flex flex-col items-start gap-4 rounded-xl border border-border bg-secondary/40 p-6 sm:p-8">
      <h2 class="text-2xl font-semibold tracking-tight">Stop checking back to find out.</h2>
      <p class="max-w-xl leading-relaxed text-muted-foreground">
        Save the next search worth watching and freehire tells you the moment — or the day — it
        matters.
      </p>
      <div class="flex flex-wrap gap-3">
        <Button href={resolve('/')} variant="primary" size="lg">Browse jobs</Button>
        <Button href={resolve('/my/notifications/settings')} variant="outline" size="lg">Notification settings</Button>
      </div>
    </div>
  </section>
</div>
