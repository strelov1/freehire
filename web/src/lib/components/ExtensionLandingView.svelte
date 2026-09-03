<script lang="ts">
  import { resolve } from '$app/paths';
  import { Button, NumberedGrid, SectionLabel } from '$lib/ui';
  import { EXTENSION_FAQ } from '$lib/extensionFaq';
  import { EXTENSION_CLAIMS, EXTENSION_STORE_URL } from '$lib/extensionLinks';

  // The three claims the panel makes, in the order it makes them on a posting:
  // it reads, it scores, it fills.

  // Autofill, as the panel actually performs it (extension/AGENTS.md). Step 03 is
  // the one that matters and the one a screenshot cannot show: the fills are
  // walked, not batched, so the audit happens while it types instead of after.
  const autofill = [
    {
      n: '01',
      title: 'Maps the real form',
      body: "Every question on the page, custom dropdowns that are not real select elements included, read off the live DOM and shown as a checklist. It counts the required ones, because those are what gate submission.",
    },
    {
      n: '02',
      title: 'Answers from your profile',
      body: 'The values come from what your freehire profile already says. Nothing is invented to fill a box, and a question your profile cannot answer stays unanswered and visible.',
    },
    {
      n: '03',
      title: 'Walks the form, in front of you',
      body: 'One question at a time, with a pause between them: the page scrolls to each field and outlines it as the value lands. The walk is the audit — you are not handed a filled form to proofread against nothing.',
    },
    {
      n: '04',
      title: 'Stops before Submit',
      body: 'The panel never presses it. What goes to the employer goes because you read the form and sent it.',
    },
  ];

  // Named as examples, never as an allowlist: the form reader works off the DOM,
  // so an unknown career page is the same case as a known vendor.
  const vendors = ['Greenhouse', 'Lever', 'Workday', 'Ashby', 'iCIMS', 'SmartRecruiters', 'Recruitee'];

  // The bounds, each of which is a property of the code rather than a policy.
  const bounds = [
    {
      key: 'panel',
      title: 'Only while the panel is open',
      body: 'The channel a page read travels over belongs to the side panel and dies with it. Close the panel and there is nothing left listening.',
    },
    {
      key: 'scheme',
      title: 'Web pages only, checked first',
      body: 'A read is refused for any tab that is not http or https, decided from the address before the page is touched — the extension is the only side that sees a URL before anything is scraped.',
    },
    {
      key: 'account',
      title: 'Yours to read and delete',
      body: 'A page the agent read stays in that conversation, on your freehire account. Read it on the web, delete it, and the panel quietly starts a fresh one.',
    },
  ];

  const start = [
    {
      n: '01',
      title: 'Add it to Chrome',
      body: 'One click from the Chrome Web Store. The panel lives behind the toolbar icon and opens beside whatever tab you are on.',
    },
    {
      n: '02',
      title: 'Sign in with freehire',
      body: 'The panel hands you to freehire to sign in once, and works from your profile after that. Free account; no key to copy anywhere.',
    },
    {
      n: '03',
      title: 'Open it on a posting',
      body: 'Any posting, on any site. The match card appears, and the agent is there to argue with about whether the job is worth the hour.',
    },
  ];
</script>

<div class="flex flex-col">
  <!-- Hero. Left: the pitch. Right: the panel, docked to a posting. -->
  <section class="dot-grid -mx-4 grid items-center gap-12 px-4 pb-16 pt-8 lg:grid-cols-[1.05fr_0.95fr]">
    <div>
      <SectionLabel text="browser extension" />
      <h1 class="mt-6 max-w-2xl text-balance text-4xl font-semibold leading-[1.0] tracking-tighter sm:text-6xl">
        Apply where you already are.
      </h1>
      <p class="mt-7 max-w-xl text-lg leading-relaxed text-muted-foreground">
        A job-application agent in Chrome's side panel, sitting next to the posting you are reading.
        It reads that page itself, tells you where you fall short of it, and fills the application
        form from your profile — on any site, not just the ones freehire tracks.
      </p>
      <div class="mt-8 flex flex-wrap gap-2">
        {#each EXTENSION_CLAIMS as claim (claim)}
          <span class="rounded-full bg-secondary px-3 py-1 text-sm font-medium text-secondary-foreground">
            {claim}
          </span>
        {/each}
      </div>
      <div class="mt-9 flex flex-wrap items-center gap-3">
        <Button href={EXTENSION_STORE_URL} target="_blank" variant="primary" size="lg">
          Add to Chrome
        </Button>
        <Button href={resolve('/jobs')} variant="outline" size="lg">Browse jobs</Button>
      </div>
      <p class="mt-4 text-sm text-muted-foreground">Free. Chrome and Chromium browsers with a side panel.</p>
    </div>

    <!-- Panel preview: a browser window with the panel docked right. Drawn, not a
         capture — a screenshot would be one theme, one employer's address bar, and
         one release out of date. -->
    <figure class="overflow-hidden rounded-xl border border-border bg-card shadow-sm">
      <figcaption class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground">
        <span class="size-2.5 rounded-full bg-muted-foreground/30"></span>
        <span class="ml-1 truncate rounded border border-border bg-background px-2 py-0.5 font-mono">
          careers.example.com/senior-backend-engineer
        </span>
      </figcaption>
      <div class="grid gap-px bg-border sm:grid-cols-[1.15fr_0.85fr]">
        <!-- The posting, as bars: it is the thing the panel is beside, not the subject. -->
        <div class="bg-background p-4">
          <div class="h-2.5 w-2/3 rounded bg-foreground/70"></div>
          <div class="mt-2 h-1.5 w-1/3 rounded bg-muted-foreground/30"></div>
          <div class="mt-5 space-y-1.5">
            <div class="h-1.5 w-full rounded bg-muted-foreground/20"></div>
            <div class="h-1.5 w-11/12 rounded bg-muted-foreground/20"></div>
            <div class="h-1.5 w-full rounded bg-muted-foreground/20"></div>
            <div class="h-1.5 w-4/5 rounded bg-muted-foreground/20"></div>
          </div>
          <div class="mt-5 space-y-1.5">
            <div class="h-1.5 w-1/4 rounded bg-muted-foreground/40"></div>
            <div class="h-1.5 w-full rounded bg-muted-foreground/20"></div>
            <div class="h-1.5 w-3/4 rounded bg-muted-foreground/20"></div>
          </div>
        </div>
        <!-- The panel. -->
        <div class="bg-background p-3">
          <div class="flex items-center gap-2 text-[11px]">
            <span class="size-4 rounded-full bg-brand"></span>
            <span class="font-semibold">freehire</span>
            <span class="ml-auto rounded-full border border-border px-1.5 py-0.5 text-muted-foreground">ready</span>
          </div>
          <div class="mt-3 flex gap-2 text-[11px]">
            <span class="rounded-full bg-secondary px-2 py-0.5 font-medium">Match</span>
            <span class="text-muted-foreground">Chat</span>
          </div>
          <div class="mt-3">
            <p class="text-2xl font-semibold tabular-nums">78%</p>
            <p class="text-[11px] text-muted-foreground">profile match</p>
            <div class="mt-3 space-y-1.5">
              <div class="h-1.5 w-full rounded bg-emerald-500/40"></div>
              <div class="h-1.5 w-4/5 rounded bg-emerald-500/40"></div>
              <div class="h-1.5 w-1/3 rounded bg-warning/40"></div>
            </div>
            <div class="mt-4 flex flex-wrap gap-1.5">
              <span class="rounded border border-emerald-400/50 px-1.5 py-0.5 font-mono text-[10px] text-emerald-600 dark:text-emerald-400">
                Kafka
              </span>
              <span class="rounded border border-emerald-400/50 px-1.5 py-0.5 font-mono text-[10px] text-emerald-600 dark:text-emerald-400">
                Postgres
              </span>
              <span class="rounded border border-warning/50 px-1.5 py-0.5 font-mono text-[10px] text-warning-strong">
                Scala
              </span>
            </div>
          </div>
        </div>
      </div>
    </figure>
  </section>

  <!-- The match card. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="before you spend the hour" />
    <div class="mt-6 grid gap-10 lg:grid-cols-2 lg:items-center">
      <div>
        <h2 class="max-w-md text-3xl font-semibold tracking-tight sm:text-4xl">
          Know whether you fit, before you apply.
        </h2>
        <p class="mt-5 max-w-md leading-relaxed text-muted-foreground">
          The panel scores the posting against your freehire profile and names both halves of the
          answer: the requirements your history covers, and the ones it does not. Not a model's
          impression of your chances — a coverage count you can read line by line and disagree with.
        </p>
        <p class="mt-5 max-w-md leading-relaxed text-muted-foreground">
          It works on a posting freehire has never seen, too. On a page outside the catalogue the
          card still scores; the actions that need a catalogue entry — saving it, running the full
          match analysis — are simply not shown rather than offered and then failing.
        </p>
        <div class="mt-8 flex flex-wrap gap-3">
          <Button href={EXTENSION_STORE_URL} target="_blank" variant="primary" size="lg">
            Add to Chrome
          </Button>
          <Button href={resolve('/features/tailor')} variant="ghost" size="lg">
            Then tailor the CV
          </Button>
        </div>
      </div>

      <!-- The card, at reading size. -->
      <figure class="overflow-hidden rounded-xl border border-border bg-card shadow-sm">
        <figcaption class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground">
          <span class="size-2.5 rounded-full bg-muted-foreground/30"></span>
          freehire · side panel
        </figcaption>
        <div class="p-5">
          <div class="flex items-baseline gap-3">
            <p class="text-4xl font-semibold tabular-nums">78%</p>
            <p class="text-sm text-muted-foreground">19 of 25 skills covered</p>
          </div>
          <div class="mt-5">
            <SectionLabel text="you have" />
            <div class="mt-2.5 flex flex-wrap gap-1.5">
              {#each ['Distributed systems', 'Kafka', 'Postgres', 'AWS', 'Go'] as skill (skill)}
                <span class="rounded-md border border-emerald-400/50 px-2 py-0.5 font-mono text-xs text-emerald-600 dark:text-emerald-400">
                  {skill}
                </span>
              {/each}
            </div>
          </div>
          <div class="mt-5">
            <SectionLabel text="they want, you don't" />
            <div class="mt-2.5 flex flex-wrap gap-1.5">
              {#each ['Scala', 'JVM tuning'] as skill (skill)}
                <span class="rounded-md border border-warning/50 px-2 py-0.5 font-mono text-xs text-warning-strong">
                  {skill}
                </span>
              {/each}
            </div>
          </div>
          <p class="mt-5 border-t border-border pt-4 text-sm leading-relaxed text-muted-foreground">
            The JVM line is the one a screener will catch. Everything else lines up.
          </p>
        </div>
      </figure>
    </div>
  </section>

  <!-- Autofill. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="autofill" />
    <div class="mt-6 max-w-2xl">
      <h2 class="text-3xl font-semibold tracking-tight sm:text-4xl">
        It reads the form and fills it in. You send it.
      </h2>
      <p class="mt-5 leading-relaxed text-muted-foreground">
        The thirty-first time you type your own phone number, the application stops being about the
        job. The panel takes that part — and does it slowly enough that you can watch, which is the
        point rather than a limitation.
      </p>
    </div>
    <NumberedGrid items={autofill} class="mt-10 sm:grid-cols-2" />

    <div class="mt-10 grid gap-10 lg:grid-cols-2 lg:items-center">
      <!-- The checklist, as the panel keeps it. -->
      <figure class="overflow-hidden rounded-xl border border-border bg-card shadow-sm">
        <figcaption class="flex items-center justify-between border-b border-border px-4 py-2.5 text-xs text-muted-foreground">
          <span>Application form</span>
          <span class="tabular-nums">32 of 33 required answered</span>
        </figcaption>
        <ul class="divide-y divide-border text-sm">
          {#each [['Full name', 'Alex Moreau'], ['Email', 'alex@example.com'], ['Years of experience', '8'], ['Work authorisation', 'EU citizen'], ['Notice period', '1 month']] as [label, value] (label)}
            <li class="flex items-baseline gap-3 px-4 py-2.5">
              <span class="text-emerald-600 dark:text-emerald-400">✓</span>
              <span class="w-40 shrink-0 text-muted-foreground">{label}</span>
              <span class="truncate">{value}</span>
            </li>
          {/each}
          <li class="flex items-baseline gap-3 bg-warning/5 px-4 py-2.5">
            <span class="text-warning-strong">•</span>
            <span class="w-40 shrink-0 text-muted-foreground">Why this company?</span>
            <span class="truncate text-muted-foreground italic">yours to write</span>
          </li>
        </ul>
      </figure>

      <div>
        <h3 class="text-2xl font-semibold tracking-tight">It knows what is not an application.</h3>
        <p class="mt-4 leading-relaxed text-muted-foreground">
          The filler only engages on a page carrying the marks of a real application — a CV upload
          among them. That is what stops a newsletter box or a job-alert signup from being written
          into on a careers page that happens to have one.
        </p>
        <p class="mt-4 leading-relaxed text-muted-foreground">
          The checklist appears on a looser test, because the screening questions on step two of an
          ATS form have no upload and are exactly where the count is most wanted. Nothing is ever
          typed on the strength of that looser test.
        </p>
      </div>
    </div>
  </section>

  <!-- The agent. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="the agent" />
    <div class="mt-6 grid gap-10 lg:grid-cols-2 lg:items-center">
      <div>
        <h2 class="max-w-md text-3xl font-semibold tracking-tight sm:text-4xl">
          Ask about the page in front of you.
        </h2>
        <p class="mt-5 max-w-md leading-relaxed text-muted-foreground">
          You do not hand the posting over, and there is no "read this page" button to remember.
          Ask whether it is a fit and the agent reads the tab itself, because the question needed it.
        </p>
        <p class="mt-5 max-w-md leading-relaxed text-muted-foreground">
          Every read is named in the conversation as it happens, so a read you did not expect is one
          you can see. The address is shown without its query or fragment — that is where session
          tokens live, and they have no business in a transcript.
        </p>
      </div>

      <!-- A turn, as the panel renders one. -->
      <figure class="overflow-hidden rounded-xl border border-border bg-card shadow-sm">
        <figcaption class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground">
          <span class="size-2.5 rounded-full bg-muted-foreground/30"></span>
          freehire · Chat
        </figcaption>
        <div class="flex flex-col gap-3 p-4 text-sm">
          <p class="ml-auto max-w-[85%] rounded-lg bg-secondary px-3 py-2 leading-relaxed">
            is this a fit? and what would they push back on?
          </p>
          <p class="font-mono text-xs text-brand">
            ⏺ read_current_page <span class="text-muted-foreground">— careers.example.com</span>
          </p>
          <p class="leading-relaxed">
            Close. They want <span class="rounded bg-secondary px-1">Scala on the JVM</span> and you have
            neither, which is the one gap a screener will catch. The rest lines up: distributed
            systems, streaming data, AWS.
          </p>
          <p class="font-mono text-xs text-brand">
            ⏺ read_profile <span class="text-muted-foreground">— your CV</span>
          </p>
          <p class="leading-relaxed">
            Your Kafka work is the closest thing you have to their throughput requirement. Lead with
            it, and say plainly that the JVM is a gap you would close.
          </p>
          <div class="mt-1 flex items-center gap-2 border-t border-border pt-3 text-xs text-muted-foreground">
            <span class="flex-1 rounded-lg border border-border px-3 py-2">Message the agent…</span>
            <span class="flex size-8 items-center justify-center rounded-full bg-brand text-brand-foreground">↑</span>
          </div>
        </div>
      </figure>
    </div>
  </section>

  <!-- Where it works. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="where it works" />
    <div class="mt-6 max-w-2xl">
      <h2 class="text-3xl font-semibold tracking-tight sm:text-4xl">On the page you are on.</h2>
      <p class="mt-5 leading-relaxed text-muted-foreground">
        Applications live on arbitrary hosts, so the panel works against the tab you opened it over
        rather than a list of sites we recognise. The form reader works off the live DOM — which
        makes an unfamiliar career page the same case as a familiar vendor, not a gap.
      </p>
    </div>
    <div class="mt-8 flex flex-wrap gap-2">
      {#each vendors as vendor (vendor)}
        <span class="rounded-full border border-border bg-background px-3.5 py-1.5 text-sm font-medium">
          {vendor}
        </span>
      {/each}
      <span class="rounded-full bg-secondary px-3.5 py-1.5 text-sm font-medium text-secondary-foreground">
        + any company career page
      </span>
    </div>
  </section>

  <!-- The bounds. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="what it can and cannot see" />
    <div class="mt-6 max-w-2xl">
      <h2 class="text-3xl font-semibold tracking-tight sm:text-4xl">
        Nothing is read in the background.
      </h2>
      <p class="mt-5 leading-relaxed text-muted-foreground">
        An extension that can see every page is worth being suspicious of. These three bounds are
        properties of how it is built, which is why they are worth stating rather than promising.
      </p>
    </div>
    <div class="mt-10 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-3">
      {#each bounds as bound (bound.key)}
        <div class="bg-background p-6 sm:p-7">
          <h3 class="text-lg font-semibold tracking-tight">{bound.title}</h3>
          <p class="mt-2 text-sm leading-relaxed text-muted-foreground">{bound.body}</p>
        </div>
      {/each}
    </div>
    <p class="mt-6 max-w-3xl text-sm leading-relaxed text-muted-foreground">
      Everything the extension sends goes to freehire and nowhere else. There is no analytics
      endpoint on the side, and no second host in the manifest.
    </p>
  </section>

  <!-- Getting started. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="getting started" />
    <NumberedGrid items={start} class="mt-10 sm:grid-cols-3" />
  </section>

  <!-- FAQ. Visible answers and the FAQPage JSON-LD share EXTENSION_FAQ. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="faq" />
    <h2 class="mt-6 max-w-md text-3xl font-semibold tracking-tight sm:text-4xl">
      Frequently asked questions.
    </h2>
    <!-- An odd entry count would leave the last row half empty, which in a hairline
         grid reads as a slab of border colour. The last answer fills the row instead. -->
    <dl
      class="mt-10 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-2 sm:[&>*:last-child:nth-child(odd)]:col-span-2"
    >
      {#each EXTENSION_FAQ as item (item.question)}
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
      <h2 class="text-2xl font-semibold tracking-tight">Put it beside the next posting you open.</h2>
      <p class="max-w-xl leading-relaxed text-muted-foreground">
        Free, and it works on whatever page you are already on — including the ones freehire does not
        track.
      </p>
      <div class="flex flex-wrap gap-3">
        <Button href={EXTENSION_STORE_URL} target="_blank" variant="primary" size="lg">
          Add to Chrome
        </Button>
        <Button href={resolve('/jobs')} variant="outline" size="lg">Browse jobs</Button>
      </div>
    </div>
  </section>
</div>
