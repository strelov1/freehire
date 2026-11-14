<script lang="ts">
  import { resolve } from '$app/paths';
  import { roleAddressExists, seniorityLabel } from '#lib/insights.js';
  import { categoryLabel } from '#lib/labels.js';
  import type { Job } from '#lib/types.js';
  import JobDescription from './JobDescription.svelte';
  import SkillChip from './SkillChip.svelte';

  // The posting itself: the model's summary, the description in the posting's own language
  // (`lang`, undefined when English), its skills and the way into the role's skill page.
  let { job, lang }: { job: Job; lang: string | undefined } = $props();

  const e = $derived(job.enrichment ?? {});
  // Both facets live under `enrichment` on the wire (jobview sets them from the
  // dictionary columns), and either can be absent.
  const roleCategory = $derived(job.enrichment?.category ?? '');
  const roleSeniority = $derived(job.enrichment?.seniority ?? '');
</script>

<!-- The posting itself. Rendered by JobView inside its Description tab panel. -->
{#if e.summary}
  <!-- Model-written synopsis (only on enriched jobs). Plain text, capped at
       400 chars server-side — the headline above the full description. -->
  <section class="flex flex-col gap-2">
    <h2 class="text-base font-semibold">Summary</h2>
    <p class="text-sm leading-relaxed">{e.summary}</p>
  </section>
{:else}
  <!-- The description's own headings are demoted to h3 (descriptionHeadings.ts) to sit
       under a section heading. Without the Summary there is none between them and the
       page's h1, so name the section for the outline: h1 → h2 → h3. -->
  <h2 class="sr-only">Description</h2>
{/if}

<!-- The Summary above is an LLM synopsis the enrichment prompt pins to English
     (internal/enrich), so the body is the only part of this snippet that takes
     the posting's language. -->
<JobDescription html={job.description} {lang} />

<!-- Some sources carry no body at all and enrichment had nothing to summarise, which
     left the Description tab blank — a reader could not tell an empty posting from a
     page that failed to load. Say so, and offer the posting's own page, which is the
     only place the rest of it can be. Not for a vacancy published here (`direct_apply`):
     it has no outside page to send anyone to. -->
{#if !e.summary && !job.description?.trim()}
  <p class="text-sm text-muted-foreground">
    The posting didn't include a description.
    {#if job.url && !job.direct_apply}
      <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- the posting's own URL on its employer's site; there is no route to resolve --><a
        href={job.url}
        target="_blank"
        rel="nofollow noopener noreferrer"
        class="font-medium text-brand-strong underline underline-offset-4"
      >
        Read it on the original site
      </a>
    {/if}
  </p>
{/if}

{#if job.skills?.length}
  <!-- Top-level `skills` is the served (deterministic-dictionary) facet; the raw
       `enrichment.skills` is kept in the JSONB and never served (see JobRow's same
       fix), so reading it here always rendered nothing. -->
  <section class="flex flex-col gap-2 border-t border-border pt-4">
    <h2 class="text-base font-semibold">Skills</h2>
    <ul class="flex flex-wrap gap-1.5">
      {#each job.skills as skill (skill)}
        <li>
          <!-- The chip used to print the raw slug, so a posting read "ci-cd" beside a
               filter panel reading "CI/CD" — the same skill spelled two ways on one
               screen. SkillChip labels it from the dictionary and, for a skill the
               glossary has reached, carries the definition too. -->
          <SkillChip slug={skill} />
        </li>
      {/each}
    </ul>
  </section>
{/if}

<!-- The way into the role's skill page, placed here because this is the moment the
     question arises: the reader has just seen what THIS posting asks for and the next
     thought is whether that is the role or just this employer. Before this the page
     existed and nothing in the product linked to it — reachable only from the footer,
     four clicks deep, from no screen where anybody is thinking about their skills.

     Shown only when both facets name a real role — the same check the route 404s on,
     so the link cannot point at a page that refuses it. Being BUSY enough to index is
     a separate question the route answers with noindex, because this page cannot know
     it without a request of its own on the hottest page on the site.

     That is a minority of postings:
     measured 2026-09-18, 98.7% of open technical postings state a category and just
     39.0% state a seniority. The link is absent rather than guessed when either is
     missing. It never 404s — a role below the publication floor is served and marked
     noindex rather than refused. -->
{#if roleAddressExists(roleCategory, roleSeniority)}
  <section class="border-t border-border pt-4">
    <a
      class="text-brand-strong hover:underline"
      href={resolve('/insights/roles/[category]/[seniority]', {
        category: roleCategory,
        seniority: roleSeniority,
      })}
    >
      What {seniorityLabel(roleSeniority)}
      {categoryLabel(roleCategory)} jobs ask for — and how much of it you have →
    </a>
  </section>
{/if}
