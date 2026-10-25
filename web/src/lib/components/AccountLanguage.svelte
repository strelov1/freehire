<script lang="ts">
  import { Check } from '@lucide/svelte';
  import { CountryFlag } from '#lib/ui/index.js';
  import { currentUser, updateLanguage } from '#lib/auth.svelte.js';
  import { ApiError } from '#lib/api.js';
  import { must } from '#lib/utils.js';
  import { SUPPORTED_LOCALES } from '#lib/locale.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t, tokenLabel } from '#lib/i18n/t.js';
  import { messages } from './AccountLanguage.messages';
  import { announce } from '#lib/announce.svelte.js';

  const s = $derived(t(messages, locale()));
  // Names the combobox after the visible heading — it has no <label> of its own.
  const headingId = $props.id();
  // Per instance, derived from the same unique id, so the listbox and its rows never
  // collide with another picker's on the page.
  const listId = `${headingId}-list`;
  const optionId = (i: number) => `${listId}-${i}`;

  // The account's preferred interface language: read from the resolved session
  // (no extra fetch — it rides GET /me already). Drives both LLM output
  // language (assistant/CV) and, for English/Russian, the translated `/my/**`
  // interface (freehire#1836) — the other four supported values still fall back
  // to English there until translated. The set is small and curated (matches
  // the backend's CHECK constraint), so a select2-style type-to-filter combobox
  // reads better here than a plain <select> with six options — flags make each
  // entry recognizable at a glance.
  //
  // The code list itself comes from `SUPPORTED_LOCALES` — the same list
  // `#lib/locale.ts` mirrors from the backend's CHECK constraint — rather than a
  // second hand-written array, so a language added or removed there needs no
  // matching edit here. Only the flag (cosmetic, has no other source of truth)
  // and the display NAME (which follows the resolved locale — see
  // AccountLanguage.messages.ts) are looked up per code.
  const FLAGS: Record<(typeof SUPPORTED_LOCALES)[number], string> = {
    en: 'gb',
    ru: 'ru',
    es: 'es',
    pt: 'pt',
    de: 'de',
    fr: 'fr',
  };

  const LANGUAGES = $derived(
    SUPPORTED_LOCALES.map((code) => ({
      code,
      flag: FLAGS[code],
      label: tokenLabel(s.languageNames, code),
    })),
  );

  type LanguageOption = (typeof LANGUAGES)[number];

  function byCode(code: string): LanguageOption {
    return LANGUAGES.find((l) => l.code === code) ?? must(LANGUAGES[0], 'default language');
  }

  function matches(lang: LanguageOption, q: string): boolean {
    const needle = q.trim().toLowerCase();
    if (!needle) return true;
    return lang.label.toLowerCase().includes(needle) || lang.code.includes(needle);
  }

  let selectedCode = $state(currentUser()?.language ?? 'en');
  const selected = $derived(byCode(selectedCode));
  let query = $state('');
  let open = $state(false);
  let activeIndex = $state(0);
  let saveState = $state<'idle' | 'saving' | 'saved' | 'error'>('idle');
  // The server's own message when it gave one; `null` means the generic fallback,
  // resolved from `s` at render time so it follows a later locale change instead
  // of freezing in whatever locale was resolved when the save failed.
  let saveErrorMessage = $state<string | null>(null);
  let savedTimer: ReturnType<typeof setTimeout> | undefined;
  let inputEl = $state<HTMLInputElement | null>(null);

  // Re-seed from the session on a real identity change (sign-in/out, or this
  // component's own save round-tripping through invalidateAll) — keyed on email
  // rather than running every render, so a pick mid-flight is never clobbered by
  // an unrelated session refresh. Unlike AccountTimezone there is nothing to
  // auto-persist: the column is never unset (defaults to "en"), so a fresh
  // account already has a value to seed from.
  let seededFor: string | null = null;
  $effect(() => {
    const user = currentUser();
    if (!user || user.email === seededFor) return;
    seededFor = user.email;
    selectedCode = user.language;
  });

  const shown = $derived(LANGUAGES.filter((l) => matches(l, query)));
  // Focus stays on the input; the highlighted row is announced through this.
  const activeDescendant = $derived(open && shown[activeIndex] ? optionId(activeIndex) : undefined);

  function openList() {
    query = '';
    open = true;
    activeIndex = 0;
  }

  function closeList() {
    open = false;
    query = '';
  }

  async function pick(lang: LanguageOption) {
    closeList();
    if (lang.code === selected.code || saveState === 'saving') return;
    const previous = selectedCode;
    selectedCode = lang.code;
    saveState = 'saving';
    saveErrorMessage = null;
    try {
      await updateLanguage(lang.code);
      saveState = 'saved';
      announce(s.saved);
      clearTimeout(savedTimer);
      savedTimer = setTimeout(() => {
        if (saveState === 'saved') saveState = 'idle';
      }, 1500);
    } catch (e) {
      selectedCode = previous;
      saveState = 'error';
      saveErrorMessage = e instanceof ApiError ? e.message : null;
      announce(saveErrorMessage ?? s.saveFailed, 'assertive');
    }
  }

  function scrollActiveIntoView() {
    document.getElementById(optionId(activeIndex))?.scrollIntoView?.({ block: 'nearest' });
  }

  function onKeydown(e: KeyboardEvent) {
    // Mid-composition (CJK/IME) the arrows and Enter belong to the IME.
    if (e.isComposing) return;
    if (!open) {
      if (e.key === 'ArrowDown' || e.key === 'Enter') {
        e.preventDefault();
        openList();
      }
      return;
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      activeIndex = Math.min(activeIndex + 1, shown.length - 1);
      scrollActiveIntoView();
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      activeIndex = Math.max(activeIndex - 1, 0);
      scrollActiveIntoView();
    } else if (e.key === 'Enter') {
      e.preventDefault();
      const active = shown[activeIndex];
      if (active) void pick(active);
    } else if (e.key === 'Escape') {
      closeList();
      inputEl?.blur();
    }
  }
</script>

<!-- One account setting's row: the heading, its save state, and the control. The card
     around it belongs to the caller, which groups this with the other account settings
     rather than boxing each one on its own. -->
<div class="flex flex-col gap-3">
  <div class="flex items-center gap-3">
    <div class="min-w-0 flex-1">
      <h2 id={headingId} class="text-sm font-semibold leading-tight">{s.heading}</h2>
      <p class="text-xs text-muted-foreground">
        {s.description}
      </p>
    </div>

    {#if saveState === 'saving'}
      <span class="text-xs text-muted-foreground">{s.saving}</span>
    {:else if saveState === 'saved'}
      <span class="flex items-center gap-1 text-xs text-brand-strong"><Check class="size-3.5" aria-hidden="true" /> {s.saved}</span>
    {:else if saveState === 'error'}
      <span class="text-xs text-destructive">{saveErrorMessage ?? s.saveFailed}</span>
    {/if}
  </div>

  <div class="relative w-full max-w-sm">
    <div class="relative">
      <span class="pointer-events-none absolute inset-y-0 left-3 flex items-center">
        <CountryFlag code={selected.flag} label={selected.label} class="text-base" />
      </span>
      <input
        bind:this={inputEl}
        type="text"
        value={open ? query : selected.label}
        oninput={(e) => {
          query = (e.currentTarget as HTMLInputElement).value;
          open = true;
          activeIndex = 0;
        }}
        onfocus={openList}
        onblur={() => setTimeout(closeList, 120)}
        onkeydown={onKeydown}
        placeholder={s.searchPlaceholder}
        autocomplete="off"
        disabled={saveState === 'saving'}
        role="combobox"
        aria-labelledby={headingId}
        aria-expanded={open}
        aria-controls={listId}
        aria-autocomplete="list"
        aria-activedescendant={activeDescendant}
        class="w-full rounded-md border border-border bg-background py-2 pl-9 pr-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      />
    </div>

    {#if open}
      <ul
        id={listId}
        role="listbox"
        class="absolute z-10 mt-1 max-h-64 w-full overflow-auto rounded-md border border-border bg-popover shadow-lg"
      >
        {#each shown as lang, i (lang.code)}
          <!-- mousedown (not click) so the pick lands before the input's blur closes the list;
               tabindex -1 keeps the rows out of the Tab order (the input owns the keyboard). -->
          <li
            id={optionId(i)}
            role="option"
            tabindex="-1"
            aria-selected={i === activeIndex}
            onmousedown={(e) => {
              e.preventDefault();
              void pick(lang);
            }}
            onmouseenter={() => (activeIndex = i)}
            class="flex w-full cursor-pointer items-center gap-3 px-3 py-2 text-left hover:bg-accent {i === activeIndex ? 'bg-accent' : ''}"
          >
            <CountryFlag code={lang.flag} label={lang.label} class="text-base" />
            <span class="truncate text-sm font-medium">{lang.label}</span>
            {#if lang.code === selected.code}
              <Check class="ml-auto size-4 shrink-0 text-brand-strong" aria-hidden="true" />
              <span class="sr-only">{s.currentChoice}</span>
            {/if}
          </li>
        {:else}
          <li role="option" aria-selected="false" aria-disabled="true" class="px-3 py-2 text-sm text-muted-foreground">
            {s.noMatches}
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</div>
