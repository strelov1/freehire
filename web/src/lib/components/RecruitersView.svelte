<script lang="ts">
  import { resolve } from '$app/paths';
  import { ArrowDown, ArrowRight } from '@lucide/svelte';
  import { RECRUITERS_FAQ } from '#lib/recruitersFaq.js';
  import { Button } from '#lib/ui/index.js';

  const vacancies = resolve('/my/employer/vacancies');

  const steps = [
    {
      title: 'Verify your company',
      body: 'Sign in and confirm your work email. Once the company is verified, you can manage its profile and vacancies.',
    },
    {
      title: 'Publish your vacancy',
      body: 'Add the role, location, and skills. Send applicants to your existing ATS or accept applications on freehire.',
    },
    {
      title: 'Review applications as a team',
      body: 'For applications submitted on freehire, open the vacancy inbox and invite recruiters to work with you.',
    },
  ];
</script>

<div class="flex flex-col gap-16 pb-8 sm:gap-24">
  <section class="grid items-center gap-8 border-b border-border pb-12 pt-8 lg:grid-cols-2 lg:gap-12 lg:pb-20 lg:pt-16">
    <div class="flex flex-col items-start gap-5">
      <h1 class="text-balance text-4xl font-semibold leading-none tracking-tight sm:text-6xl">
        Post tech jobs. Manage applicants.
      </h1>
      <p class="max-w-md text-lg leading-relaxed text-muted-foreground">
        Publish your vacancy for free. Accept applications on freehire to review résumés, track hiring stages, and email
        candidates from one shared workspace.
      </p>
      <div class="flex flex-wrap items-center gap-x-5 gap-y-3">
        <Button href={vacancies} variant="primary" size="lg">
          Post a job for free <ArrowRight class="size-4" aria-hidden="true" />
        </Button>
        <a
          href="#hiring-workspace"
          class="inline-flex min-h-11 items-center gap-2 text-sm font-medium underline-offset-4 hover:underline focus-visible:underline"
        >
          See the workspace <ArrowDown class="size-4" aria-hidden="true" />
        </a>
      </div>
    </div>

    <figure class="min-w-0 rounded-xl bg-muted p-3 sm:p-5">
      <img
        src="/recruiters/applications.webp"
        alt="The application inbox with a candidate search, filters by hiring stage, and an unread application."
        width="794"
        height="264"
        class="block h-auto w-full rounded-lg border border-border"
        fetchpriority="high"
      />
      <figcaption class="mt-3 text-xs text-muted-foreground">Application inbox · sample data</figcaption>
    </figure>
  </section>

  <section id="hiring-workspace" class="scroll-mt-24" aria-labelledby="workspace-heading">
    <h2 id="workspace-heading" class="text-balance text-3xl font-semibold tracking-tight sm:text-4xl">
      A shared inbox for your hiring team
    </h2>
    <p class="mt-3 max-w-2xl leading-relaxed text-muted-foreground">
      Choose on-site applications to give your hiring team a single place to review candidates and follow up.
    </p>
    <dl class="mt-7 grid gap-x-8 gap-y-6 sm:grid-cols-3">
      <div class="border-t border-border pt-4">
        <dt class="font-semibold">Review candidates</dt>
        <dd class="mt-2 text-sm leading-relaxed text-muted-foreground">
          Search and filter applications, read screening answers, download submitted résumés, and update hiring stages.
        </dd>
      </div>
      <div class="border-t border-border pt-4">
        <dt class="font-semibold">Work as a team</dt>
        <dd class="mt-2 text-sm leading-relaxed text-muted-foreground">
          Invite recruiters, leave private notes, and see changes in your company’s activity log.
        </dd>
      </div>
      <div class="border-t border-border pt-4">
        <dt class="font-semibold">Keep the conversation together</dt>
        <dd class="mt-2 text-sm leading-relaxed text-muted-foreground">
          Email candidates from the application. Their replies appear in the same thread, with a badge for new
          responses.
        </dd>
      </div>
    </dl>
  </section>

  <section class="grid items-center gap-6 lg:grid-cols-5 lg:gap-12" aria-labelledby="analytics-heading">
    <div class="lg:col-span-2">
      <h2 id="analytics-heading" class="text-balance text-3xl font-semibold tracking-tight sm:text-4xl">
        See how each vacancy performs
      </h2>
      <p class="mt-3 max-w-xl leading-relaxed text-muted-foreground">
        See daily unique views, clicks on Apply, and applications delivered through freehire over the last 30 completed
        days. The report also shows clicks per view.
      </p>
    </div>
    <figure class="min-w-0 rounded-xl bg-muted p-3 sm:p-5 lg:col-span-3">
      <img
        src="/recruiters/analytics.webp"
        alt="The vacancy report showing daily unique views, Apply clicks, applications via freehire, and clicks per view."
        width="886"
        height="218"
        class="block h-auto w-full rounded-lg border border-border"
        loading="lazy"
      />
      <figcaption class="mt-3 text-xs text-muted-foreground">Vacancy report · sample data</figcaption>
    </figure>
  </section>

  <section aria-labelledby="getting-started-heading">
    <h2 id="getting-started-heading" class="text-3xl font-semibold tracking-tight">How to get started</h2>
    <ol class="mt-6 grid list-decimal gap-x-10 gap-y-5 pl-5 marker:font-semibold sm:grid-cols-3">
      {#each steps as step (step.title)}
        <li class="pl-1">
          <h3 class="font-semibold">{step.title}</h3>
          <p class="mt-1 text-sm leading-relaxed text-muted-foreground">{step.body}</p>
        </li>
      {/each}
    </ol>
    <div
      class="mt-8 flex flex-col items-start gap-3 border-t border-border pt-5 sm:flex-row sm:items-center sm:justify-between"
    >
      <p class="max-w-xl text-sm leading-relaxed text-muted-foreground">
        Already have an external job link? Submit it for moderation, or browse the catalogue to see how listed jobs
        look.
      </p>
      <div class="flex flex-wrap items-center gap-3">
        <Button href={resolve('/submit')} variant="outline">Submit an existing job</Button>
        <Button href={resolve('/jobs')} variant="ghost">Browse jobs</Button>
      </div>
    </div>
  </section>

  <section
    class="flex flex-col items-start gap-4 border-y border-border py-6 sm:flex-row sm:items-center sm:justify-between"
    aria-labelledby="promotion-pilot-heading"
  >
    <div>
      <h2 id="promotion-pilot-heading" class="text-xl font-semibold tracking-tight">Job promotion pilot</h2>
      <p class="mt-1 text-sm leading-relaxed text-muted-foreground">
        Have an open role that needs more visibility? Tell us about it.
      </p>
    </div>
    <Button href={resolve('/recruiters/job-promotion')} variant="outline">
      Explore the pilot <ArrowRight class="size-4" aria-hidden="true" />
    </Button>
  </section>

  <section class="flex flex-col gap-5" aria-labelledby="recruiter-questions-heading">
    <h2 id="recruiter-questions-heading" class="text-3xl font-semibold tracking-tight">Questions</h2>
    <dl class="grid gap-x-10 sm:grid-cols-2">
      {#each RECRUITERS_FAQ as item (item.question)}
        <div class="border-t border-border py-4">
          <dt class="font-semibold">{item.question}</dt>
          <dd class="mt-2 text-sm leading-relaxed text-muted-foreground">{item.answer}</dd>
        </div>
      {/each}
    </dl>
  </section>

  <section
    class="flex flex-col items-start gap-5 rounded-xl bg-muted p-6 sm:flex-row sm:items-center sm:justify-between sm:p-8"
  >
    <h2 class="text-2xl font-semibold tracking-tight">Ready to fill your next tech role?</h2>
    <Button href={vacancies} variant="primary" size="lg">
      Post a job for free <ArrowRight class="size-4" aria-hidden="true" />
    </Button>
  </section>
</div>
