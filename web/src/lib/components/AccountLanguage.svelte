<script lang="ts">
  import { Check, Globe } from '@lucide/svelte';
  import { CountryFlag } from '$lib/ui';
  import { currentUser, updateLanguage } from '$lib/auth.svelte';
  import { ApiError } from '$lib/api';
  import { must } from '$lib/utils';

  // The account's preferred interface language: read from the resolved session
  // (no extra fetch — it rides GET /me already). No UI translation ships yet —
  // this just records the preference, ahead of interface i18n and
  // assistant/CV language behavior (freehire#1836). The set is small and
  // curated (matches the backend's CHECK constraint), so a select2-style
  // type-to-filter combobox reads better here than a plain <select> with six
  // options — flags make each entry recognizable at a glance.
  const LANGUAGES = [
    { code: 'en', label: 'English', flag: 'gb' },
    { code: 'ru', label: 'Russian', flag: 'ru' },
    { code: 'es', label: 'Spanish', flag: 'es' },
    { code: 'pt', label: 'Portuguese', flag: 'pt' },
    { code: 'de', label: 'German', flag: 'de' },
    { code: 'fr', label: 'French', flag: 'fr' },
  ] as const;

  type LanguageOption = (typeof LANGUAGES)[number];

  function byCode(code: string): LanguageOption {
    return LANGUAGES.find((l) => l.code === code) ?? must(LANGUAGES[0], 'default language');
  }

  function matches(lang: LanguageOption, q: string): boolean {
    const needle = q.trim().toLowerCase();
    if (!needle) return true;
    return lang.label.toLowerCase().includes(needle) || lang.code.includes(needle);
  }

  let selected = $state(byCode(currentUser()?.language ?? 'en'));
  let query = $state('');
  let open = $state(false);
  let activeIndex = $state(0);
  let saveState = $state<'idle' | 'saving' | 'saved' | 'error'>('idle');
  let saveError = $state<string | null>(null);
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
    selected = byCode(user.language);
  });

  const shown = $derived(LANGUAGES.filter((l) => matches(l, query)));

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
    const previous = selected;
    selected = lang;
    saveState = 'saving';
    saveError = null;
    try {
      await updateLanguage(lang.code);
      saveState = 'saved';
      clearTimeout(savedTimer);
      savedTimer = setTimeout(() => {
        if (saveState === 'saved') saveState = 'idle';
      }, 1500);
    } catch (e) {
      selected = previous;
      saveState = 'error';
      saveError = e instanceof ApiError ? e.message : 'Could not save.';
    }
  }

  function onKeydown(e: KeyboardEvent) {
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
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      activeIndex = Math.max(activeIndex - 1, 0);
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

<section class="rounded-xl border border-border bg-card p-4">
  <div class="flex items-center gap-3">
    <div class="grid size-9 shrink-0 place-items-center rounded-lg bg-brand-muted text-brand-strong">
      <Globe class="size-4.5" aria-hidden="true" />
    </div>
    <div class="min-w-0 flex-1">
      <h2 class="text-sm font-semibold leading-tight">Language</h2>
      <p class="text-xs text-muted-foreground">
        Your preferred language for the assistant and CV once interface translations ship.
      </p>
    </div>

    {#if saveState === 'saving'}
      <span class="text-xs text-muted-foreground">Saving…</span>
    {:else if saveState === 'saved'}
      <span class="flex items-center gap-1 text-xs text-brand-strong"><Check class="size-3.5" aria-hidden="true" /> Saved</span>
    {:else if saveState === 'error'}
      <span class="text-xs text-destructive">{saveError}</span>
    {/if}
  </div>

  <div class="relative mt-4 w-full max-w-sm border-t border-border pt-4">
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
        placeholder="Search a language…"
        autocomplete="off"
        disabled={saveState === 'saving'}
        role="combobox"
        aria-expanded={open}
        aria-controls="language-picker-list"
        class="w-full rounded-md border border-border bg-background py-2 pl-9 pr-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      />
    </div>

    {#if open}
      <ul
        id="language-picker-list"
        role="listbox"
        class="absolute z-10 mt-1 max-h-64 w-full overflow-auto rounded-md border border-border bg-popover shadow-lg"
      >
        {#each shown as lang, i (lang.code)}
          <li>
            <!-- mousedown (not click) so the pick lands before the input's blur closes the list -->
            <button
              type="button"
              role="option"
              aria-selected={lang.code === selected.code}
              onmousedown={(e) => {
                e.preventDefault();
                void pick(lang);
              }}
              onmouseenter={() => (activeIndex = i)}
              class="flex w-full items-center gap-3 px-3 py-2 text-left hover:bg-accent {i === activeIndex ? 'bg-accent' : ''}"
            >
              <CountryFlag code={lang.flag} label={lang.label} class="text-base" />
              <span class="truncate text-sm font-medium">{lang.label}</span>
              {#if lang.code === selected.code}
                <Check class="ml-auto size-4 shrink-0 text-brand-strong" aria-hidden="true" />
              {/if}
            </button>
          </li>
        {:else}
          <li class="px-3 py-2 text-sm text-muted-foreground">No matches</li>
        {/each}
      </ul>
    {/if}
  </div>
</section>
