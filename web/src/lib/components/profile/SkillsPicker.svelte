<script lang="ts">
  // The Skills chip/search UI shared by first-time set-up (ProfileForm, local unsaved state
  // until the form's own Save) and the steady-state Skills view (autosaving on every toggle,
  // via profileStore). This component holds no opinion on persistence — it reports toggles
  // through `onToggleSkill` and shows whatever `skills` the caller currently holds; only the
  // skill dictionary (the typeahead's universe) is loaded here.
  //
  // Skills to avoid live in AvoidCard.svelte (the Avoid part of the Skills tab), not here —
  // they used to render as a second block in this component, but that coupled "what I have"
  // and "what I avoid" into one control that only Skills needed to reach.
  import { loadSkillDistribution, searchSkillOptions } from '#lib/skillDictionary.js';
  import type { FacetOption } from '#lib/facets.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import type { SaveStatus } from '#lib/saveStatus.svelte.js';
  import { messages } from './SkillsPicker.messages';
  import RemoteSearchSelect from '../facets/RemoteSearchSelect.svelte';
  import SaveStatusLine from '../SaveStatus.svelte';

  const s = $derived(t(messages, locale()));

  let {
    skills,
    onToggleSkill,
    status,
  }: {
    skills: string[];
    onToggleSkill: (skill: string) => void;
    /** The autosaving Skills view's save state, shown on the label row (and dimming the
     *  picker while a write is in flight). Set-up saves with the whole form, so it passes
     *  none. */
    status?: SaveStatus;
  } = $props();

  const busy = $derived(status?.saving ?? false);

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
    return Promise.resolve(searchSkillOptions(skillDist, query));
  }
</script>

<div class="flex flex-col gap-2 {busy ? 'pointer-events-none opacity-60' : ''}">
  <div class="flex items-baseline justify-between">
    <span class="text-sm font-medium">{s.heading}</span>
    <span class="flex items-baseline gap-3">
      {#if status}<SaveStatusLine {status} />{/if}
      <span class="text-xs tabular-nums text-muted-foreground">{skills.length}</span>
    </span>
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
