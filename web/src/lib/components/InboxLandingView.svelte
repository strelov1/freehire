<script lang="ts">
  import { resolve } from '$app/paths';
  import { Button } from '$lib/ui';
  import { NumberedGrid, SectionLabel } from '$lib/ui';
  import StatusChip from '$lib/components/StatusChip.svelte';
  import { INBOX_FAQ } from '$lib/inboxFaq';
  import { INBOX_STATUS_GUIDE } from '$lib/inboxStatusGuide';

  // Illustrative inbox — decorative, not live data. Every sender is an ATS relay
  // of the kind that really writes (the matcher deliberately never trusts those
  // domains; it reads the thread and the company name instead), and every status
  // is a real `mailclassify` signal rendered with the product's own chip.
  const mail = [
    {
      from: 'Avenga Careers',
      subject: 'We have received your application!',
      when: 'yesterday',
      signal: 'acknowledgement',
      unread: true,
    },
    {
      from: 'Fingerprint Recruiting',
      subject: 'Regarding your application to Fingerprint',
      when: '2 days ago',
      signal: 'rejection',
    },
    {
      from: 'Gabrielle Loureiro',
      subject: 'Interview for Senior Full-Stack Engineer',
      when: '2 days ago',
      signal: 'interview_invitation',
    },
    {
      from: 'Pack.com Recruiting',
      subject: 'A few questions about your experience',
      when: '3 days ago',
      signal: 'info_request',
    },
  ];

  // The two messages hanging off one application in the board preview: the ATS
  // acknowledgement that opened the thread, and the reply that moved the card.
  const linked = [
    {
      from: 'no-reply@us.greenhouse-mail.io',
      subject: 'Thank you for applying to Speechify',
      signal: 'acknowledgement',
    },
    {
      from: 'Gabrielle Loureiro',
      subject: 'Interview for Senior Full-Stack Engineer',
      signal: 'interview_invitation',
    },
  ];

  // The stage ladder an email can push a card along — `mailclassify.stageOrder`,
  // in its own order. Terminal outcomes are absent on purpose: nothing automatic
  // ever moves a card into or out of one.
  const ladder = ['Applied', 'Screening', 'Responded', 'Interview', 'Offer'];

  const promises = [
    {
      n: '01',
      title: 'Only the mail you point at us',
      body: 'The freehire address receives what you forward and what employers reply to — nothing else. Connect Gmail instead and the sync is read-only and scoped to job mail: freehire learns which senders write about your applications and leaves the rest of your mailbox alone.',
    },
    {
      n: '02',
      title: 'A guess is a suggestion, not a decision',
      body: "An email is linked on its own only when the match is certain — the same thread, or the company's name in the sender or subject. The AI classifier can propose a match, but its pick waits for your confirmation. Read the asymmetry the other way: model output never silently rewrites your board.",
    },
    {
      n: '03',
      title: 'Leave whenever, in one click',
      body: 'Disconnect Gmail and the sync stops. Release the freehire address and it stops receiving. Delete a message and it stays deleted — a later re-sync will not resurrect it, because read, deleted and triaged are your state, not the mail server’s.',
    },
    {
      n: '04',
      title: 'Open, so you can check',
      body: 'The classifier, the matcher and the stage rules are in the repository. Every claim on this page is a file you can read — including the one that says an out-of-vocabulary answer is stored as “other” instead of being taken at face value.',
    },
  ];
</script>

<!-- The two marks the previews repeat: the initial tile that stands in for a
     sender's avatar, and the status chip in the product's own colours. -->
{#snippet initial(name: string, size: string)}
  <div
    class="grid {size} shrink-0 place-items-center rounded-lg border border-border font-mono font-medium"
  >
    {name.charAt(0).toUpperCase()}
  </div>
{/snippet}

<div class="flex flex-col">
  <!-- Hero. Left: the pitch. Right: the inbox itself, chips and all. -->
  <section class="dot-grid -mx-4 grid items-center gap-12 px-4 pb-16 pt-8 lg:grid-cols-[1.05fr_0.95fr]">
    <div>
      <SectionLabel text="inbox" />
      <h1 class="mt-6 max-w-2xl text-balance text-4xl font-semibold leading-[1.0] tracking-tighter sm:text-6xl">
        Your recruiter replies, sorted automatically.
      </h1>
      <p class="mt-7 max-w-xl text-lg leading-relaxed text-muted-foreground">
        Job mail arrives in one place, tagged with what it actually says — received, rejected,
        interview, information requested — and attached to the application it belongs to. No folders,
        no digging through a mailbox to work out where you stand.
      </p>
      <div class="mt-9 flex flex-wrap items-center gap-3">
        <Button href={resolve('/my/inbox')} variant="primary" size="lg">Open your inbox</Button>
        <Button href={resolve('/my/tracking')} variant="outline" size="lg">See your board</Button>
      </div>
    </div>

    <figure class="overflow-hidden rounded-xl border border-border bg-card shadow-sm">
      <figcaption class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground">
        <span class="size-2.5 rounded-full bg-muted-foreground/30"></span>
        freehire · Inbox
      </figcaption>
      <ul class="divide-y divide-border">
        {#each mail as m (m.subject)}
          <li class="flex items-start gap-3 p-4">
            {@render initial(m.from, 'size-9 text-sm')}
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                {#if m.unread}
                  <span class="size-1.5 shrink-0 rounded-full bg-brand" aria-label="unread"></span>
                {/if}
                <p class="truncate text-sm font-medium">{m.from}</p>
                <span class="ml-auto shrink-0 text-xs text-muted-foreground">{m.when}</span>
              </div>
              <p class="mt-0.5 truncate text-sm text-muted-foreground">{m.subject}</p>
              <span class="mt-2 inline-block">
                <StatusChip signal={m.signal} class="text-[10px] leading-4" />
              </span>
            </div>
          </li>
        {/each}
      </ul>
    </figure>
  </section>

  <!-- Connect. The hosted address leads because it works for everyone; Gmail is
       second while its OAuth app is still in Google review (test users only). -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="connect" />
    <div class="mt-6 max-w-2xl">
      <h2 class="text-3xl font-semibold tracking-tight sm:text-4xl">Two ways to get mail in.</h2>
      <p class="mt-5 leading-relaxed text-muted-foreground">
        Pick either, or run both — they feed the same inbox.
      </p>
    </div>

    <div class="mt-10 grid gap-6 lg:grid-cols-2">
      <div class="flex flex-col gap-4 rounded-xl border border-border bg-secondary/40 p-6">
        <div class="flex flex-col gap-1">
          <span class="font-mono text-sm text-muted-foreground">Recommended</span>
          <h3 class="text-lg font-semibold tracking-tight">Claim a freehire address</h3>
        </div>
        <p class="text-sm leading-relaxed text-muted-foreground">
          You get an address like <code
            class="rounded bg-background/70 px-1.5 py-0.5 font-mono text-xs text-foreground">you@mail.freehire.me</code
          >. Use it when you apply and the replies land here directly, or forward the job mail you
          already have. Nothing else reaches it, and you can release it at any time.
        </p>
      </div>

      <div class="flex flex-col gap-4 rounded-xl border border-border p-6">
        <div class="flex flex-col gap-1">
          <span class="font-mono text-sm text-muted-foreground">Or</span>
          <h3 class="text-lg font-semibold tracking-tight">Connect Gmail, read-only</h3>
        </div>
        <p class="text-sm leading-relaxed text-muted-foreground">
          Sign in with Google once and freehire syncs the job-related mail it finds — it learns the
          senders that write about your applications rather than reading your whole mailbox.
          Disconnecting stops the sync immediately. Google is still reviewing the app, so this route
          is open to test users for now.
        </p>
      </div>
    </div>
  </section>

  <!-- Statuses. Rendered from the product's own vocabulary and chip styles. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="statuses" />
    <div class="mt-6 max-w-2xl">
      <h2 class="text-3xl font-semibold tracking-tight sm:text-4xl">It reads each message and tags it.</h2>
      <p class="mt-5 leading-relaxed text-muted-foreground">
        Every message is classified into one controlled vocabulary — the same eight labels the inbox
        filters on. Anything that doesn't clearly fit is stored as
        <code class="font-mono text-foreground">other</code>, never as a guess dressed up as a fact.
      </p>
    </div>

    <dl class="mt-10 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-2">
      {#each INBOX_STATUS_GUIDE as s (s.signal)}
        <!-- The chip column is fixed-width so the descriptions line up down the
             grid instead of stepping in and out with each label's length. -->
        <div class="flex items-baseline gap-3 bg-background p-5 sm:p-6">
          <dt class="w-28 shrink-0"><StatusChip signal={s.signal} /></dt>
          <dd class="text-sm leading-relaxed text-muted-foreground">{s.description}</dd>
        </div>
      {/each}
    </dl>
  </section>

  <!-- The board. Where the mail stops being mail and becomes your pipeline. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="on your board" />
    <div class="mt-6 max-w-2xl">
      <h2 class="text-3xl font-semibold tracking-tight sm:text-4xl">
        Every reply lands on the application it belongs to.
      </h2>
      <p class="mt-5 leading-relaxed text-muted-foreground">
        A message is attached automatically when the match is certain — the same thread, or the
        company's name in the sender or the subject. Otherwise it waits as a suggestion you confirm
        with one click. Either way the correspondence for a role ends up in one place, on the card,
        instead of scattered across a mailbox.
      </p>
      <div class="mt-8 flex flex-wrap gap-3">
        <Button href={resolve('/my/tracking')} variant="primary" size="lg">Open Tracking</Button>
        <Button href={resolve('/my/inbox')} variant="ghost" size="lg">Go to the inbox</Button>
      </div>
    </div>

    <figure class="mt-10 overflow-hidden rounded-xl border border-border bg-card shadow-sm">
      <figcaption class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground">
        <span class="size-2.5 rounded-full bg-muted-foreground/30"></span>
        My jobs · Application
      </figcaption>
      <div class="p-5 sm:p-6">
        <div class="flex flex-wrap items-center gap-3">
          {@render initial('Speechify', 'size-10 text-sm')}
          <div class="min-w-0">
            <p class="truncate font-medium">Tech Lead, Web Core Product</p>
            <p class="truncate text-sm text-muted-foreground">Speechify</p>
          </div>
          <span class="ml-auto"><StatusChip signal="interview_invitation" /></span>
        </div>

        <div class="mt-5 flex items-center gap-2 border-b border-border pb-3 text-sm">
          <span class="text-muted-foreground">Application</span>
          <span class="text-muted-foreground">Job Match</span>
          <span class="rounded-full bg-secondary px-2.5 py-1 font-medium">Emails (2)</span>
        </div>

        <ul class="mt-4 flex flex-col gap-2">
          {#each linked as m (m.subject)}
            <li class="flex items-start gap-3 rounded-lg border border-border bg-background p-3">
              {@render initial(m.from, 'size-8 text-xs')}
              <div class="min-w-0 flex-1">
                <p class="truncate text-sm">{m.from}</p>
                <p class="truncate text-xs text-muted-foreground">{m.subject}</p>
              </div>
              <span class="shrink-0"><StatusChip signal={m.signal} class="text-[10px] leading-4" /></span>
            </li>
          {/each}
        </ul>
      </div>
    </figure>

    <!-- The stage ladder, and the two rules that keep it honest. The ladder gets
         a full row of its own so it never wraps mid-arrow. -->
    <div class="mt-10 flex flex-col gap-6">
      <ol class="flex flex-wrap items-center gap-x-2 gap-y-3">
        {#each ladder as stage, i (stage)}
          <li class="flex items-center gap-2">
            <span class="rounded-full border border-border px-3 py-1 text-sm">{stage}</span>
            {#if i < ladder.length - 1}
              <span class="text-muted-foreground" aria-hidden="true">→</span>
            {/if}
          </li>
        {/each}
      </ol>
      <p class="max-w-3xl text-sm leading-relaxed text-muted-foreground">
        A confident reply nudges the card forward along that ladder, and only forward. It never walks
        a card back, never touches an application you already settled as rejected, accepted or
        withdrawn, and a rejection email never moves a card by itself — the outcome stays yours to
        record.
      </p>
    </div>
  </section>

  <!-- Privacy: the objection this feature has to answer before anything else. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="what we don't do" />
    <div class="mt-6 max-w-2xl">
      <h2 class="text-3xl font-semibold tracking-tight sm:text-4xl">
        Handing over your mail is a big ask.
      </h2>
      <p class="mt-5 leading-relaxed text-muted-foreground">
        So here is the shape of it, written down.
      </p>
    </div>
    <NumberedGrid items={promises} class="mt-10 sm:grid-cols-2" />
  </section>

  <!-- Agents: the third, unmetered tier. Mirrors HomeView's terminal figure. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="for agents" />
    <div class="mt-6 grid gap-10 lg:grid-cols-2 lg:items-center">
      <div>
        <h2 class="max-w-md text-3xl font-semibold tracking-tight sm:text-4xl">
          Bring your own mail client.
        </h2>
        <p class="mt-5 max-w-md leading-relaxed text-muted-foreground">
          The whole inbox is in the freehire CLI, so your own client — himalaya, mbsync, anything
          that speaks IMAP — can fetch the mail and hand it over with
          <code class="font-mono text-foreground">inbox push</code>. Each message is keyed by its
          Message-ID, so a nightly re-sync updates rather than duplicates. freehire stores it, links
          it and shows it on the board like any other message, but never classifies it: that tier
          costs nothing to run and nothing to use.
        </p>
        <p class="mt-4 max-w-md leading-relaxed text-muted-foreground">
          <code class="font-mono text-foreground">inbox list --unclassified --body</code> is then
          your agent's work queue — a whole page of messages to judge in one call, and unlike
          <code class="font-mono text-foreground">inbox read</code> it marks nothing read.
          <code class="font-mono text-foreground">inbox triage</code> records the verdict and moves
          the application's stage.
        </p>
        <div class="mt-8 flex flex-wrap gap-3">
          <Button href={resolve('/cli')} variant="primary" size="lg">Get the CLI</Button>
          <Button href={resolve('/docs/api')} variant="ghost" size="lg">API reference</Button>
        </div>
      </div>

      <figure class="overflow-hidden rounded-xl border border-border bg-secondary/60 font-mono text-sm shadow-sm">
        <figcaption class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground">
          <span class="size-2.5 rounded-full bg-muted-foreground/30"></span>
          terminal
        </figcaption>
        <pre class="overflow-x-auto p-4 leading-relaxed"><span class="text-muted-foreground"># hand over a batch your client fetched (external_id = Message-ID,</span>
<span class="text-muted-foreground"># so re-pushing updates instead of duplicating)</span>
freehire <span class="text-foreground">inbox push --file mail.json</span>

<span class="text-muted-foreground"># the work queue: unjudged mail, bodies inline, nothing marked read</span>
freehire <span class="text-foreground">inbox list --unclassified --body</span>

<span class="text-muted-foreground"># record the verdict; the application's stage follows</span>
freehire <span class="text-foreground">inbox triage &lt;id&gt; interview_invitation --slug &lt;job&gt;</span>

<span class="text-muted-foreground"># the queues the matcher won't guess at</span>
freehire <span class="text-foreground">inbox list --link suggested</span>   <span class="text-muted-foreground"># confirm / reject</span>
freehire <span class="text-foreground">inbox list --link unlinked</span>    <span class="text-muted-foreground"># inbox application</span></pre>
      </figure>
    </div>
  </section>

  <!-- FAQ. The visible answers and the FAQPage JSON-LD share INBOX_FAQ. -->
  <section class="border-t border-border py-16 sm:py-20">
    <SectionLabel text="faq" />
    <h2 class="mt-6 max-w-md text-3xl font-semibold tracking-tight sm:text-4xl">
      Frequently asked questions.
    </h2>
    <dl class="mt-10 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-2">
      {#each INBOX_FAQ as item (item.question)}
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
      <h2 class="text-2xl font-semibold tracking-tight">Stop guessing where you stand.</h2>
      <p class="max-w-xl leading-relaxed text-muted-foreground">
        Claim your freehire address, apply with it, and watch the replies sort themselves onto your
        board. It's free, like the rest of freehire.
      </p>
      <div class="flex flex-wrap gap-3">
        <Button href={resolve('/my/inbox')} variant="primary" size="lg">Open your inbox</Button>
        <Button href={resolve('/')} variant="outline" size="lg">Find jobs to apply to</Button>
      </div>
    </div>
  </section>
</div>
