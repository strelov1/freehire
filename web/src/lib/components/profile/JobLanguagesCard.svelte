<script lang="ts">
  import { onMount } from 'svelte';
  import { Check, ChevronDown, Globe, Languages, X } from '@lucide/svelte';
  import { api } from '#lib/api.js';
  import { dynamicLabel, languageFlag, type FacetOption } from '#lib/facets.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';
  import { profileStore } from '#lib/profile.svelte.js';
  import { SaveStatus } from '#lib/saveStatus.svelte.js';
  import { ComboboxNav } from '#lib/comboboxNav.svelte.js';
  import { CountryFlag, Input } from '#lib/ui/index.js';
  import SaveStatusLine from '../SaveStatus.svelte';
  import { messages } from './JobLanguagesCard.messages';

  let { onProfileChanged }: { onProfileChanged?: () => void } = $props();
  const s = $derived(t(messages, locale()));
  const selected = $derived(profileStore.profile?.job_languages ?? []);
  const status = new SaveStatus();
  let available = $state<string[]>([]);
  let loading = $state(true);
  let query = $state('');
  const fieldId = $props.id();
  let loadFailed = $state(false);
  const languageNames = $derived(new Intl.DisplayNames([locale()], { type: 'language' }));
  const options = $derived<FacetOption[]>(
    available
      .map((value) => ({
        value,
        label: languageLabel(value),
        flag: languageFlag(value),
      }))
      .sort((a, b) => a.label.localeCompare(b.label, locale())),
  );
  const shown = $derived(
    options.filter(
      (option) =>
        option.label.toLocaleLowerCase(locale()).includes(query.trim().toLocaleLowerCase(locale())) ||
        option.value.includes(query.trim().toLowerCase()),
    ),
  );
  const nav = new ComboboxNav<FacetOption>({
    id: fieldId,
    items: () => shown,
    loading: () => loading,
    pick: (option) => toggle(option.value),
  });

  /** Resolve a posting-language code in the account locale. */
  function languageLabel(code: string): string {
    return languageNames.of(code) ?? dynamicLabel('posting_language', code);
  }

  /** Load the available choices from the unfiltered live job-language facet. */
  async function loadLanguages() {
    loadFailed = false;
    loading = true;
    try {
      const counts = await api.facetCounts(new URLSearchParams(), { facets: ['posting_language'] });
      available = Object.entries(counts.facets.posting_language ?? {})
        .filter(([, count]) => count > 0)
        .map(([code]) => code);
    } catch {
      loadFailed = true;
    } finally {
      loading = false;
    }
  }
  onMount(() => {
    void loadLanguages();
  });

  /** Persist one selection change, then refresh profile-derived search alerts. */
  function toggle(value: string) {
    if (status.saving) return;
    nav.close();
    query = '';
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
  <div class="relative">
    <Input
      bind:value={query}
      placeholder={loading ? s.loading : selected.length ? s.search : s.anyLanguage}
      aria-label={s.heading}
      role="combobox"
      aria-expanded={nav.open}
      aria-controls={nav.listId}
      aria-autocomplete="list"
      aria-activedescendant={nav.activeDescendant}
      autocomplete="off"
      disabled={status.saving}
      class="w-full pr-9"
      onfocus={() => (nav.open = true)}
      onclick={() => (nav.open = true)}
      oninput={() => {
        nav.open = true;
        nav.active = -1;
      }}
      onblur={nav.close}
      onkeydown={(event) => {
        if (event.key === 'Escape' && !event.isComposing && nav.open) {
          event.preventDefault();
          event.stopPropagation();
          nav.close();
        } else nav.onKeydown(event);
      }}
    />
    <ChevronDown
      aria-hidden="true"
      class="pointer-events-none absolute right-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
    />
    {#if nav.open}
      <ul
        id={nav.listId}
        role="listbox"
        aria-label={s.heading}
        aria-multiselectable="true"
        aria-busy={loading}
        class="absolute inset-x-0 top-full z-10 mt-1 max-h-60 overflow-y-auto rounded-md border border-border bg-popover p-1 shadow-lg"
      >
        {#each shown as option, i (option.value)}
          <li
            id={nav.optionId(i)}
            role="option"
            tabindex="-1"
            aria-selected={selected.includes(option.value)}
            onmouseenter={() => (nav.active = i)}
            onmousedown={(event) => event.preventDefault()}
            onclick={() => toggle(option.value)}
            onkeydown={(event) => {
              if (event.key === 'Enter' || event.key === ' ') {
                event.preventDefault();
                toggle(option.value);
              }
            }}
            class="flex cursor-pointer items-center gap-2 rounded-md px-3 py-2 text-sm hover:bg-accent {nav.active === i
              ? 'bg-accent'
              : ''}"
          >
            <span aria-hidden="true">
              {#if option.flag}<CountryFlag code={option.flag} label={option.label} />{:else}<Globe
                  class="size-4"
                />{/if}
            </span>
            <span>{option.label}</span>
            {#if selected.includes(option.value)}<Check
                aria-hidden="true"
                class="ml-auto size-4 text-brand-strong"
              />{/if}
          </li>
        {:else}
          <li role="option" aria-selected="false" aria-disabled="true" class="px-3 py-2 text-sm text-muted-foreground">
            {loading ? s.loading : loadFailed ? s.loadFailed : s.noMatches}
          </li>
        {/each}
      </ul>
    {/if}
  </div>
  {#if selected.length}
    <div class="flex flex-wrap gap-2">
      {#each selected as code (code)}
        {@const flag = languageFlag(code)}
        <button
          type="button"
          disabled={status.saving}
          aria-label={format(s.remove, { language: languageLabel(code) })}
          onclick={() => toggle(code)}
          class="inline-flex items-center gap-1.5 rounded-full border border-border px-2.5 py-1 text-sm hover:bg-accent disabled:opacity-60"
        >
          <span aria-hidden="true"
            >{#if flag}<CountryFlag code={flag} label={languageLabel(code)} />{:else}<Globe class="size-4" />{/if}</span
          >
          {languageLabel(code)}<X aria-hidden="true" class="size-3.5" />
        </button>
      {/each}
    </div>
  {/if}
  {#if loadFailed}
    <p role="status" class="text-sm text-muted-foreground">
      {s.loadFailed}
      <button type="button" class="underline" onclick={loadLanguages}>{s.retry}</button>
    </p>
  {/if}
</section>
