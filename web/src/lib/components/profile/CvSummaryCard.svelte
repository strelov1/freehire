<script lang="ts">
  // Read-only projection of the structured CV — headline, summary, the CV-stated location
  // (a different fact from the job-search "where you're based" preference on the Location
  // view — see internal/resume/AGENTS.md's three-layer table) and languages. Education and
  // certifications have their own view (EducationCard) rather than living here. None of
  // this has a write path today (internal/resumeextract stores it read-only, regenerated
  // wholesale on every CV re-upload) — the only way to change it is a new upload, which is
  // why there is no edit control here; the upload/replace control sits right above this
  // card instead.
  //
  // Renders nothing until there is something to show — no separate "still parsing"
  // placeholder, since a CV can sit pending indefinitely (no LLM configured, a stuck
  // job) and a permanent-looking stuck message reads as broken rather than as progress.
  import { Languages } from '@lucide/svelte';
  import type { Professional } from '$lib/generated/contracts';
  import { Chip } from '$lib/ui';

  let { cv }: { cv: Professional | null } = $props();

  const languages = $derived(cv?.languages ?? []);
  const hasAnything = $derived(
    Boolean(cv?.headline || cv?.summary || cv?.location || languages.length),
  );
</script>

{#if hasAnything}
  <div class="flex flex-col gap-4">
    <h2 class="text-sm font-semibold">Your CV</h2>

    {#if cv?.headline}
      <p class="text-sm font-medium">{cv.headline}</p>
    {/if}

    {#if cv?.summary}
      <p class="text-sm leading-relaxed">{cv.summary}</p>
    {/if}

    {#if cv?.location}
      <p class="text-xs text-muted-foreground">As stated on your CV: {cv.location}</p>
    {/if}

    {#if languages.length}
      <div class="flex flex-col gap-2">
        <h3 class="flex items-center gap-2 text-xs font-medium text-muted-foreground">
          <Languages class="size-3.5" />Languages
        </h3>
        <div class="flex flex-wrap gap-1.5">
          {#each languages as lang, i (i)}
            <Chip>{lang}</Chip>
          {/each}
        </div>
      </div>
    {/if}
  </div>
{/if}
