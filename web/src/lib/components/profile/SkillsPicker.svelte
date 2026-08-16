<script lang="ts">
  // Skills + Skills-to-avoid, the chip/search UI shared by first-time set-up (ProfileForm,
  // local unsaved state until the form's own Save) and the steady-state Skills view
  // (autosaving on every toggle, via profileStore). This component holds no opinion on
  // persistence — it reports toggles through `onToggleSkill`/`onToggleExcluded` and shows
  // whatever `skills`/`excludedSkills` the caller currently holds; only the skill dictionary
  // (the typeahead's universe) is loaded here, since every caller needs the same one.
  import { loadSkillDistribution } from '$lib/skillDictionary';
  import type { FacetOption } from '$lib/facets';
  import RemoteSearchSelect from '../facets/RemoteSearchSelect.svelte';

  let {
    skills,
    excludedSkills,
    onToggleSkill,
    onToggleExcluded,
    busy = false,
  }: {
    skills: string[];
    excludedSkills: string[];
    onToggleSkill: (skill: string) => void;
    onToggleExcluded: (skill: string) => void;
    busy?: boolean;
  } = $props();

  let skillDist = $state.raw<FacetOption[]>([]);
  // See RemoteSearchSelect's `ready` prop: without it, a dictionary fetch slower than the
  // picker's 250ms debounce leaves the popular first page stuck empty.
  let skillDistReady = $state(false);

  $effect(() => {
    void loadSkillDistribution().then((dist) => {
      skillDist = dist;
      skillDistReady = true;
    });
  });

  function searchSkillsExcept(query: string, avoid: string[]): Promise<FacetOption[]> {
    const q = query.trim().toLowerCase();
    const pool = skillDist.filter((o) => !avoid.includes(o.value));
    const matches = q ? pool.filter((o) => o.label.toLowerCase().includes(q)) : pool;
    return Promise.resolve(matches.slice(0, q ? 50 : 8));
  }

  const searchSkills = (query: string) => searchSkillsExcept(query, excludedSkills);
  const searchExcludedSkills = (query: string) => searchSkillsExcept(query, skills);
</script>

<div class="flex flex-col gap-4 {busy ? 'pointer-events-none opacity-60' : ''}">
  <div class="flex flex-col gap-2">
    <div class="flex items-baseline justify-between">
      <span class="text-sm font-medium">Skills</span>
      <span class="text-xs tabular-nums text-muted-foreground">{skills.length}</span>
    </div>
    <RemoteSearchSelect
      search={searchSkills}
      include={skills}
      placeholder="Search skills"
      onToggle={onToggleSkill}
      fallbackLabel={(v) => v}
      clearOnSelect
      ready={skillDistReady}
    />
  </div>

  <!-- Kept disjoint from Skills (a skill can't be both wanted and avoided — the server
       enforces this too, dropping any overlap). Passed as the control's `exclude` set so the
       chips render in the destructive (red, struck-through) style, matching how an excluded
       facet value looks everywhere else. -->
  <div class="flex flex-col gap-2">
    <div class="flex items-baseline justify-between">
      <span class="text-sm font-medium">Skills to avoid</span>
      <span class="text-xs tabular-nums text-muted-foreground">{excludedSkills.length}</span>
    </div>
    <RemoteSearchSelect
      search={searchExcludedSkills}
      include={[]}
      exclude={excludedSkills}
      placeholder="Search skills to exclude"
      onToggle={onToggleExcluded}
      fallbackLabel={(v) => v}
      clearOnSelect
      ready={skillDistReady}
    />
    <span class="text-xs text-muted-foreground">
      Filtered out when you apply your profile to the job filters.
    </span>
  </div>
</div>
