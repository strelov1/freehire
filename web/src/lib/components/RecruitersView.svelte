<script lang="ts">
  import { Button } from '$lib/ui';

  // Why a recruiter should submit. Kept honest — these are properties of the
  // actual flow (free, moderated, developer audience), not marketing promises.
  const benefits = [
    {
      title: 'Free forever',
      body: 'freehire is an open-source, non-commercial aggregator. Posting a job costs nothing — no fees, no paywall, no upsell.',
    },
    {
      title: 'Reviewed, not spammed',
      body: 'Every submission is read by a moderator before it goes live, so the catalogue stays clean and your posting sits among real roles.',
    },
    {
      title: 'A developer audience',
      body: 'Your role lands in one normalized feed alongside openings from Greenhouse, Lever and Ashby — searchable by stack, seniority and location.',
    },
  ];

  const steps = [
    { n: '01', title: 'Submit', body: 'Paste the job URL and a few details. Sign in first — it takes a moment.' },
    { n: '02', title: 'Review', body: 'A moderator checks the posting. You can follow its status under “My submissions”.' },
    { n: '03', title: 'Live', body: 'Once approved, the role joins the public catalogue and search — enriched and deduplicated like every other source.' },
  ];
</script>

<div class="flex flex-col gap-16">
  <!-- Hero -->
  <section class="dot-grid -mx-4 flex flex-col gap-7 px-4 pb-4 pt-2">
    <p class="font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">
      // for recruiters &amp; hiring teams
    </p>
    <h1 class="max-w-2xl text-balance text-4xl font-semibold leading-[1.0] tracking-tighter sm:text-6xl">
      Put your role in front of developers.
    </h1>
    <p class="max-w-xl text-lg leading-relaxed text-muted-foreground">
      Submit a vacancy to freehire and, once a moderator approves it, it joins one clean, searchable
      feed of tech jobs — free, open-source, no fees.
    </p>
    <div class="flex flex-wrap items-center gap-3">
      <Button href="/submit" variant="primary" size="lg">Submit a job</Button>
      <Button href="/" variant="outline" size="lg">Browse the catalogue</Button>
    </div>
  </section>

  <!-- Why -->
  <section class="grid gap-6 sm:grid-cols-3">
    {#each benefits as b (b.title)}
      <div class="flex flex-col gap-2 rounded-lg border border-border p-5">
        <h2 class="text-base font-semibold tracking-tight">{b.title}</h2>
        <p class="text-sm leading-relaxed text-muted-foreground">{b.body}</p>
      </div>
    {/each}
  </section>

  <!-- How it works -->
  <section class="flex flex-col gap-6">
    <h2 class="text-2xl font-semibold tracking-tight">How it works</h2>
    <ol class="grid gap-6 sm:grid-cols-3">
      {#each steps as s (s.n)}
        <li class="flex flex-col gap-2">
          <span class="font-mono text-sm text-muted-foreground">{s.n}</span>
          <span class="text-base font-semibold tracking-tight">{s.title}</span>
          <span class="text-sm leading-relaxed text-muted-foreground">{s.body}</span>
        </li>
      {/each}
    </ol>
  </section>

  <!-- Closing CTA -->
  <section class="flex flex-col items-start gap-4 rounded-lg border border-border bg-secondary/40 p-6">
    <h2 class="text-xl font-semibold tracking-tight">Ready to post a role?</h2>
    <p class="max-w-xl text-sm leading-relaxed text-muted-foreground">
      Submitting takes a minute. You’ll need an account so you can track the review.
    </p>
    <Button href="/submit" variant="primary" size="lg">Submit a job</Button>
  </section>
</div>
