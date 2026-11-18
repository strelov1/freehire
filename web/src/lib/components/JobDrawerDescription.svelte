<script lang="ts">
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import type { Job } from '#lib/types.js';
  import JobDescription from './JobDescription.svelte';
  import SkillChip from './SkillChip.svelte';
  import { messages } from './JobDrawer.messages';

  // The drawer's Description tab: the posting's text and skills, or why there is none.
  let {
    removed,
    posting,
  }: {
    /** The posting was removed from the catalogue. */
    removed: boolean;
    /** The posting, once loaded. */
    posting: Job | null;
  } = $props();

  const s = $derived(t(messages, locale()));
</script>

<div class="flex flex-col gap-5">
  {#if removed}
    <!-- The posting was removed from the catalogue. What it said is genuinely
         gone; saying so is better than an empty tab that reads as a bug. -->
    <p class="text-sm text-muted-foreground">
      {s.postingRemoved}
    </p>
  {:else if posting?.description}
    <JobDescription html={posting.description} />
  {:else if !posting}
    <p class="text-sm text-muted-foreground">{s.loading}</p>
  {:else}
    <p class="text-sm text-muted-foreground">{s.noDescription}</p>
  {/if}

  {#if posting?.skills?.length}
    <div class="flex flex-col gap-2 border-t border-border pt-5">
      <p class="text-sm font-medium text-muted-foreground">{s.skills}</p>
      <div class="flex flex-wrap gap-1.5">
        <!-- Unlinked: the drawer is a reading surface over a posting, and a
             filter link would navigate out of it. -->
        {#each posting.skills as skill (skill)}
          <SkillChip slug={skill} linked={false} />
        {/each}
      </div>
    </div>
  {/if}
</div>
