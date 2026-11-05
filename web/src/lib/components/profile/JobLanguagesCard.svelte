<script lang="ts">
  import { onMount } from 'svelte';
  import { Languages } from '@lucide/svelte';
  import { api } from '#lib/api.js';
  import { dynamicLabel, type FacetOption } from '#lib/facets.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { profileStore } from '#lib/profile.svelte.js';
  import { SaveStatus } from '#lib/saveStatus.svelte.js';
  import SearchSelect from '../facets/SearchSelect.svelte';
  import SaveStatusLine from '../SaveStatus.svelte';
  import { messages } from './JobLanguagesCard.messages';

  let { onProfileChanged }: { onProfileChanged?: () => void } = $props();
  const s = $derived(t(messages, locale()));
  const selected = $derived(profileStore.profile?.job_languages ?? []);
  const status = new SaveStatus();
  let available = $state<string[]>([]);
  let loadFailed = $state(false);
  const languageNames = $derived(new Intl.DisplayNames([locale()], { type: 'language' }));
  const options = $derived<FacetOption[]>(
    [...new Set([...selected, ...available])].map((value) => ({
      value,
      label: languageNames.of(value) ?? dynamicLabel('posting_language', value),
    })),
  );

  async function loadLanguages() {
    loadFailed = false;
    try {
      const counts = await api.facetCounts(new URLSearchParams(), { facets: ['posting_language'] });
      available = Object.keys(counts.facets.posting_language ?? {});
    } catch {
      loadFailed = true;
    }
  }
  onMount(() => { void loadLanguages(); });

  function toggle(value: string) {
    if (status.saving) return;
    const next = selected.includes(value) ? selected.filter((v) => v !== value) : [...selected, value];
    void status.run(async () => {
      await profileStore.updateJobLanguages(next);
      onProfileChanged?.();
    });
  }
</script>

<section id="job-languages" class="mt-8 flex scroll-mt-20 flex-col gap-4 border-t border-border pt-6">
  <div class="flex items-baseline justify-between gap-3">
    <h2 class="flex items-center gap-2 text-base font-semibold">
      <Languages class="size-4.5" />{s.heading}
    </h2>
    <SaveStatusLine {status} />
  </div>
  <p class="text-sm text-muted-foreground">{s.description}</p>
  <div inert={status.saving} class={status.saving ? 'opacity-60' : ''}>
    <SearchSelect {options} include={selected} onToggle={toggle} placeholder={s.search} clearOnSelect />
  </div>
  {#if loadFailed}
    <p role="status" class="text-sm text-muted-foreground">
      {s.loadFailed}
      <button type="button" class="underline" onclick={loadLanguages}>{s.retry}</button>
    </p>
  {/if}
</section>
