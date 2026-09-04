<script lang="ts">
  // The skills typeahead, dictionary-backed like SkillsPicker — but skills only, with no
  // "avoid" half: excluding a skill is a refinement for someone who already has a feed, not
  // a question to ask before they have seen one.
  import RemoteSearchSelect from '$lib/components/facets/RemoteSearchSelect.svelte';
  import { loadSkillDistribution } from '$lib/skillDictionary';
  import type { FacetOption } from '$lib/facets';
  import { X } from '@lucide/svelte';

  interface Props {
    skills: string[];
    onChange: (next: string[]) => void;
  }

  let { skills, onChange }: Props = $props();

  let skillDist = $state.raw<FacetOption[]>([]);
  let skillDistReady = $state(false);
  $effect(() => {
    void loadSkillDistribution().then((dist) => {
      skillDist = dist;
      skillDistReady = true;
    });
  });

  function searchSkills(query: string): Promise<FacetOption[]> {
    const q = query.trim().toLowerCase();
    const matches = q ? skillDist.filter((o) => o.label.toLowerCase().includes(q)) : skillDist;
    return Promise.resolve(matches.slice(0, q ? 50 : 8));
  }

  function toggleSkill(value: string) {
    onChange(skills.includes(value) ? skills.filter((v) => v !== value) : [...skills, value]);
  }
</script>

<h2 class="text-xl font-semibold tracking-tight">What are your skills?</h2>
<p class="mt-1 text-sm text-muted-foreground">Optional — search and add as many as apply.</p>

<div class="mt-5">
  <div class="mb-2 flex min-h-6 items-center justify-between gap-2">
    <span class="text-sm font-medium">Skills</span>
    {#if skills.length > 0}
      <button
        type="button"
        onclick={() => onChange([])}
        title="Clear Skills"
        aria-label="Clear Skills"
        class="flex size-5 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
      >
        <X class="size-3.5" />
      </button>
    {/if}
  </div>
</div>
<RemoteSearchSelect
  search={searchSkills}
  include={skills}
  placeholder="Search skills"
  onToggle={toggleSkill}
  fallbackLabel={(v) => v}
  clearOnSelect
  ready={skillDistReady}
  techIcons
/>
