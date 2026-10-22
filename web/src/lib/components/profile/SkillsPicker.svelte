<script lang="ts">
  // The Skills chip/search UI shared by first-time set-up (ProfileForm, local unsaved state
  // until the form's own Save) and the steady-state Skills view (autosaving on every toggle,
  // via profileStore). This component holds no opinion on persistence — it reports toggles
  // through `onToggleSkill` and shows whatever `skills` the caller currently holds; only the
  // skill dictionary (the typeahead's universe) is loaded here.
  //
  // Skills to avoid live on the dedicated Avoid tab (AvoidCard.svelte), not here — they used
  // to render as a second block in this component, but that coupled "what I have" and "what
  // I avoid" into one control that only Skills needed to reach.
  import { loadSkillDistribution } from '#lib/skillDictionary.js';
  import type { FacetOption } from '#lib/facets.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { messages } from './SkillsPicker.messages';
  import RemoteSearchSelect from '../facets/RemoteSearchSelect.svelte';

  const s = $derived(t(messages, locale()));

  let {
    skills,
    onToggleSkill,
    busy = false,
  }: {
    skills: string[];
    onToggleSkill: (skill: string) => void;
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

  function searchSkills(query: string): Promise<FacetOption[]> {
    const q = query.trim().toLowerCase();
    const matches = q ? skillDist.filter((o) => o.label.toLowerCase().includes(q)) : skillDist;
    return Promise.resolve(matches.slice(0, q ? 50 : 8));
  }
</script>

<div class="flex flex-col gap-2 {busy ? 'pointer-events-none opacity-60' : ''}">
  <div class="flex items-baseline justify-between">
    <span class="text-sm font-medium">{s.heading}</span>
    <span class="text-xs tabular-nums text-muted-foreground">{skills.length}</span>
  </div>
  <RemoteSearchSelect
    search={searchSkills}
    include={skills}
    placeholder={s.searchPlaceholder}
    onToggle={onToggleSkill}
    fallbackLabel={(v) => v}
    clearOnSelect
    ready={skillDistReady}
    techIcons
  />
</div>
