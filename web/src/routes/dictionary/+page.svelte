<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { SvelteURLSearchParams } from 'svelte/reactivity';
  import { Briefcase, Tags } from '@lucide/svelte';
  import Seo from '#lib/components/Seo.svelte';
  import { TabStrip, tabStripId } from '#lib/ui/index.js';
  import CategoryPane from '#lib/components/filters/CategoryPane.svelte';
  import SearchSelect from '#lib/components/facets/SearchSelect.svelte';
  import { ApiError, api } from '#lib/api.js';
  import { dynamicOptions } from '#lib/facets.js';
  import { filtersToParams } from '#lib/filters.js';
  import { promptSignIn } from '#lib/signin.js';
  import { StagedFilters } from '#lib/stagedFilters.svelte.js';

  type Kind = 'specialization' | 'skill';

  let { data } = $props();

  const store = new StagedFilters();
  // Seeded from the URL so a visitor sent to sign in comes back to the term they typed.
  let specQuery = $state(page.url.searchParams.get('specialization') ?? '');
  let skillQuery = $state(page.url.searchParams.get('skill') ?? '');
  // A skill term (back from sign-in, or handed over by the filter) or ?tab=skill opens Skills.
  let tab = $state<Kind>(
    page.url.searchParams.has('skill') || page.url.searchParams.get('tab') === 'skill' ? 'skill' : 'specialization',
  );
  const tabs = [
    { id: 'specialization', label: 'Specialization', icon: Briefcase },
    { id: 'skill', label: 'Skills', icon: Tags },
  ] as const;
  const PANEL_ID = 'dictionary-panel';

  // Per term: how many times it has been asked for, 'known' when the server already
  // resolves it, or a refusal to show.
  let asked = $state<Record<string, number | 'known' | 'limit' | 'failed'>>({});
  let pending = $state<string | null>(null);

  const key = (kind: Kind, term: string) => `${kind}:${term.toLowerCase()}`;

  const skills = $derived(store.facet('skills'));
  // Every skill the dictionary knows, busiest first; without counts, no numbers.
  const skillOptions = $derived.by(() => {
    const options = dynamicOptions('skills', data.counts?.facets.skills ?? {}, [...data.skillSlugs, ...skills.include]);
    return data.counts ? options : options.map((o) => ({ ...o, count: undefined }));
  });
  const jobsQuery = $derived(store.active > 0 ? filtersToParams(store.value).toString() : '');

  async function rememberTerm(kind: Kind, term: string) {
    const params = new SvelteURLSearchParams();
    params.set(kind, term);
    const next = `${resolve('/dictionary')}?${params}`;
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- query is appended to resolve('/dictionary')
    await goto(next, { replaceState: true, reset: false });
  }

  async function add(kind: Kind, term: string) {
    const k = key(kind, term);
    pending = k;
    try {
      const res = await api.recordDictionaryGap(kind, term);
      asked[k] = res.request_count;
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        await rememberTerm(kind, term);
        promptSignIn();
        return;
      }
      if (err instanceof ApiError && err.status === 409) asked[k] = 'known';
      else if (err instanceof ApiError && err.status === 429) asked[k] = 'limit';
      else asked[k] = 'failed';
    } finally {
      pending = null;
    }
  }
</script>

<Seo
  title="Help us recognize your role · freehire"
  description="Every specialization and skill freehire recognizes in job posts. Can't find your role? Add it and help us recognize it."
  canonical={`${page.url.origin}/dictionary`}
/>

{#snippet missing(kind: Kind, term: string)}
  {@const state = asked[key(kind, term)]}
  <div class="flex flex-wrap items-center gap-3 py-1 text-sm">
    {#if state === undefined}
      <span>“{term}” is not in the dictionary.</span>
      <button
        type="button"
        class="rounded-full bg-foreground px-3 py-1 font-medium text-background disabled:opacity-50"
        disabled={pending === key(kind, term)}
        onclick={() => void add(kind, term)}
      >
        Add to the list
      </button>
    {:else if state === 'known'}
      <span class="text-muted-foreground">“{term}” is already in the dictionary under another name.</span>
    {:else if state === 'limit'}
      <span class="text-muted-foreground">Daily limit reached. Try again tomorrow.</span>
    {:else if state === 'failed'}
      <span class="text-muted-foreground">Could not record this term.</span>
    {:else}
      <span class="text-muted-foreground">Thanks. Asked {state} {state === 1 ? 'time' : 'times'}.</span>
    {/if}
  </div>
{/snippet}

{#snippet missingSpecialization(term: string)}{@render missing('specialization', term)}{/snippet}
{#snippet missingSkill(term: string)}{@render missing('skill', term)}{/snippet}

<div class="mx-auto w-full max-w-4xl px-4 py-6">
  <header class="mb-6">
    <h1 class="text-2xl font-semibold tracking-tight">Can't find your role? Help us recognize it</h1>
    <p class="mt-2 max-w-2xl text-sm leading-relaxed text-muted-foreground">
      These are the specializations and skills we recognize in job posts. Search for yours. If it isn't here, add it and we'll teach the dictionary.
    </p>
  </header>

  <TabStrip {tabs} active={tab} onSelect={(id) => (tab = id)} label="Dictionary" panelId={PANEL_ID} class="mb-6" />

  <section id={PANEL_ID} role="tabpanel" aria-labelledby={tabStripId(PANEL_ID, tab)} tabindex="-1">
    {#if tab === 'specialization'}
      <CategoryPane {store} plain counts={data.counts} bind:query={specQuery} empty={missingSpecialization} />
    {:else}
      <h2 class="mb-2 text-sm font-semibold tracking-tight">Skills</h2>
      <SearchSelect
        options={skillOptions}
        include={skills.include}
        placeholder="Search skills"
        techIcons
        expand
        onToggle={(v) => store.pick('skills', v)}
        bind:filter={skillQuery}
        empty={missingSkill}
      />
    {/if}
  </section>

  {#if jobsQuery}
    <div class="sticky bottom-4 mt-8 flex justify-end">
      <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- internal /jobs filter link; resolve()d base plus a query string -->
      <a href={`${resolve('/jobs')}?${jobsQuery}`} class="rounded-full bg-foreground px-4 py-2 text-sm font-medium text-background shadow">
        Show jobs
      </a>
    </div>
  {/if}
</div>
