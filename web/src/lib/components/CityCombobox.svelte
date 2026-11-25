<script lang="ts">
  // A text field that suggests "City, Country" as you type (api.searchCities). It owns the
  // typeahead — debounce, stale-response guard, ARIA combobox — and nothing else: the text
  // is `value` (bindable, so a value set from outside, such as a link prefill, simply shows),
  // and picking a suggestion reports the structured row through `onPick`. Shared by the
  // profile's "where you're based" field and the employer vacancy form.
  import { api } from '#lib/api.js';
  import { ComboboxNav } from '#lib/comboboxNav.svelte.js';
  import { countryLabel } from '#lib/facets.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { Input } from '#lib/ui/index.js';
  import { messages } from './CityCombobox.messages';

  type CityRow = { value: string; country: string; label: string };

  let {
    id,
    value = $bindable(''),
    placeholder,
    onInput,
    onPick,
  }: {
    id: string;
    value?: string;
    placeholder?: string;
    /** Fires on every keystroke, before the search: typing invalidates an earlier pick. */
    onInput?: () => void;
    onPick?: (row: CityRow) => void;
  } = $props();

  const s = $derived(t(messages, locale()));

  let results = $state.raw<CityRow[]>([]);
  let loading = $state(false);
  let reqToken = 0;
  let timer: ReturnType<typeof setTimeout> | undefined;
  // svelte-ignore state_referenced_locally
  const nav = new ComboboxNav({ id, items: () => results, loading: () => loading, pick });

  function handleInput() {
    onInput?.();
    nav.open = true;
    nav.active = -1;
    clearTimeout(timer);
    const q = value.trim();
    if (q.length < 2) {
      reqToken++;
      results = [];
      loading = false;
      return;
    }
    loading = true;
    const mine = ++reqToken;
    timer = setTimeout(async () => {
      try {
        const rows = await api.searchCities(q);
        if (mine !== reqToken) return;
        results = rows.map((r) => ({
          value: r.value,
          country: r.country,
          label: `${r.value}, ${countryLabel(r.country)}`,
        }));
      } catch {
        if (mine !== reqToken) return;
        results = [];
      } finally {
        if (mine === reqToken) {
          loading = false;
          nav.active = -1;
        }
      }
    }, 250);
  }

  function pick(row: CityRow) {
    value = row.label;
    nav.close();
    results = [];
    onPick?.(row);
  }
</script>

<div class="relative">
  <Input
    {id}
    bind:value
    oninput={handleInput}
    onfocus={() => value.trim().length >= 2 && (nav.open = true)}
    onblur={() => setTimeout(nav.close, 120)}
    onkeydown={nav.onKeydown}
    {placeholder}
    autocomplete="off"
    role="combobox"
    aria-expanded={nav.listOpen}
    aria-controls={nav.listId}
    aria-autocomplete="list"
    aria-activedescendant={nav.activeDescendant}
    class="w-full"
  />
  {#if nav.listOpen}
    <ul
      id={nav.listId}
      role="listbox"
      class="absolute inset-x-0 top-full z-10 mt-1 max-h-60 overflow-y-auto rounded-md border border-border bg-popover p-1 shadow-lg"
    >
      {#if loading && results.length === 0}
        <li role="option" aria-selected="false" aria-disabled="true" class="px-2 py-1.5 text-sm text-muted-foreground">
          {s.searching}
        </li>
      {/if}
      {#each results as row, i (row.value + row.country)}
        <!-- mousedown (not click) so the pick lands before the input's blur closes the
             list; tabindex -1 keeps the rows out of the Tab order. -->
        <li
          id={nav.optionId(i)}
          role="option"
          tabindex="-1"
          aria-selected={nav.active === i}
          onmouseenter={() => (nav.active = i)}
          onmousedown={(e) => {
            e.preventDefault();
            pick(row);
          }}
          class="flex w-full cursor-pointer items-center rounded-md px-2 py-1.5 text-left text-sm hover:bg-accent {nav.active ===
          i
            ? 'bg-accent'
            : ''}"
        >
          {row.label}
        </li>
      {/each}
    </ul>
  {/if}
</div>
