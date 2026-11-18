<script lang="ts">
  import { skillLabel } from '#lib/facets.js';
  import { Badge } from '#lib/ui/index.js';
  import SkillIcon from './SkillIcon.svelte';

  // A job card's skill chips: the first few (`shownSkills`) and a count of the rest
  // (`extraSkills`). `haveSet` is the viewer's own skills, lowercased, when there is a
  // profile to compare against.
  let {
    shownSkills,
    extraSkills,
    haveSet,
  }: { shownSkills: string[]; extraSkills: number; haveSet: Set<string> | null } = $props();

  // A chip is red when the viewer's own profile lacks the skill — the same source as the
  // strip below, so the chips and the percentage can never tell different stories. With
  // no profile to compare against there is nothing to tint: the chips are plain facts.
  function chipVariant(skill: string): 'brand' | 'missing' | 'outline' {
    if (haveSet) return haveSet.has(skill.toLowerCase()) ? 'brand' : 'missing';
    return 'outline';
  }
</script>

<div class="mt-3 flex min-w-0 flex-wrap items-center gap-1.5">
  <!-- A long dictionary slug ("event-driven-architecture") would wrap inside its
       own chip and stretch the row to two lines, breaking the card's rhythm on a
       phone. Each chip stays one line and truncates with an ellipsis past max-w; the full skill is
       in `title` for anyone who needs it. -->
  <!-- Use the same labels as the filters. Chips stay display-only under the
         stretched title link; definitions live on the posting itself. -->
  {#each shownSkills as skill (skill)}
    <Badge variant={chipVariant(skill)} class="max-w-[9rem] gap-1">
      <SkillIcon slug={skill} />
      <span class="truncate" title={skillLabel(skill)}>{skillLabel(skill)}</span>
    </Badge>
  {/each}
  {#if extraSkills > 0}
    <span class="text-xs text-muted-foreground">+{extraSkills} skills</span>
  {/if}
</div>
