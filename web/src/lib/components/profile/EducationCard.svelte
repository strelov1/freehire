<script lang="ts">
  // Education + certifications — read-only, from the structured CV (see CvSummaryCard's
  // doc comment for why: no write path exists for these fields today, only a re-upload).
  import { Award, GraduationCap } from '@lucide/svelte';
  import type { Professional } from '$lib/generated/contracts';
  import { Chip } from '$lib/ui';

  let { cv }: { cv: Professional | null } = $props();

  const education = $derived(cv?.education ?? []);
  const certifications = $derived(cv?.certifications ?? []);
</script>

{#if education.length === 0 && certifications.length === 0}
  <p class="text-sm text-muted-foreground">
    Nothing here yet — add education and certifications to your CV and re-upload it in Profile.
  </p>
{:else}
  <div class="flex flex-col gap-6">
    {#if education.length}
      <div class="flex flex-col gap-2">
        <h2 class="flex items-center gap-2 text-sm font-semibold">
          <GraduationCap class="size-4" />Education
        </h2>
        <ul class="flex flex-col gap-1.5">
          {#each education as ed, i (i)}
            <li class="flex flex-wrap items-baseline justify-between gap-2 rounded-xl border border-border bg-card p-4 text-sm">
              <span>
                <span class="font-medium">{ed.degree || ed.institution}</span>
                {#if ed.degree && ed.institution}
                  <span class="text-muted-foreground"> · {ed.institution}</span>
                {/if}
              </span>
              {#if ed.year}
                <span class="text-xs tabular-nums text-muted-foreground">{ed.year}</span>
              {/if}
            </li>
          {/each}
        </ul>
      </div>
    {/if}

    {#if certifications.length}
      <div class="flex flex-col gap-2">
        <h2 class="flex items-center gap-2 text-sm font-semibold">
          <Award class="size-4" />Certifications
        </h2>
        <div class="flex flex-wrap gap-1.5">
          {#each certifications as cert, i (i)}
            <Chip>{cert}</Chip>
          {/each}
        </div>
      </div>
    {/if}
  </div>
{/if}
