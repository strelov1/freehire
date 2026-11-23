<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { ArrowRight, FileText, NotebookPen } from '@lucide/svelte';
  import Seo from '#lib/components/Seo.svelte';

  const description = 'Free tools to help you evaluate jobs and plan your next application.';
</script>

<Seo title="Job search tools · Freehire" {description} canonical={page.url.origin + resolve('/tools')} />

<div class="mx-auto w-full max-w-5xl px-4 py-10 sm:px-6 sm:py-14">
  <h1 class="text-3xl font-semibold tracking-tight sm:text-4xl">Job search tools</h1>
  <p class="mt-3 text-muted-foreground">{description}</p>

  <ul class="mt-8 divide-y divide-border border-y border-border">
    <li>
      <a
        href={resolve('/roast')}
        class="group flex items-start gap-4 py-5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:items-center"
      >
        <FileText class="mt-0.5 size-5 shrink-0 text-brand sm:mt-0" aria-hidden="true" />
        <div class="min-w-0 flex-1">
          <h2 class="font-semibold group-hover:underline">Roast my CV</h2>
          <p class="mt-1 text-sm text-muted-foreground">
            Upload a PDF to check your CV’s ATS readiness and see how many open roles your skills reach. No account needed.
          </p>
        </div>
        <ArrowRight class="mt-0.5 size-4 shrink-0 text-muted-foreground sm:mt-0" aria-hidden="true" />
      </a>
    </li>
    <li>
      <a
        href={resolve('/tools/interview-prep')}
        class="group flex items-start gap-4 py-5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:items-center"
      >
        <NotebookPen class="mt-0.5 size-5 shrink-0 text-brand sm:mt-0" aria-hidden="true" />
        <div class="min-w-0 flex-1">
          <h2 class="font-semibold group-hover:underline">Interview prep</h2>
          <p class="mt-1 text-sm text-muted-foreground">
            Practice questions from a job posting and find relevant examples for your answers.
          </p>
        </div>
        <ArrowRight class="mt-0.5 size-4 shrink-0 text-muted-foreground sm:mt-0" aria-hidden="true" />
      </a>
    </li>
  </ul>
</div>
