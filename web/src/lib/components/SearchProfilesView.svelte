<script lang="ts">
  import { ApiError, extractResumeSkills, facetCounts } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { CATEGORY_OPTIONS, categoryLabel, type FacetOption } from '$lib/facets';
  import { searchProfiles } from '$lib/searchProfiles.svelte';
  import { categoryParams, computeGap, sortSkillsByCount, type SkillGap } from '$lib/skillGap';
  import type { SearchProfile } from '$lib/types';
  import { Button, Input } from '$lib/ui';
  import RemoteSearchSelect from './facets/RemoteSearchSelect.svelte';
  import SearchSelect from './facets/SearchSelect.svelte';
  import States from './States.svelte';

  // Mirror of the server's specialization cap (searchprofile.maxSpecializations).
  const MAX_SPECIALIZATIONS = 5;

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  const profiles = $derived(searchProfiles.items);

  // The universe of skills (canonical tokens with job counts) for the typeahead, fetched
  // from the facet-distribution endpoint — the same source the filter panel's dynamic
  // skills facet uses. Empty params = the whole catalogue's skill distribution.
  let skillDist = $state.raw<FacetOption[]>([]);

  // Form doubles as create (editingId null) and edit (editingId set). A profile needs a
  // name, at least one specialization, and at least one skill — the same invariants the
  // server enforces.
  let editingId = $state<number | null>(null);
  let name = $state('');
  let specializations = $state.raw<string[]>([]);
  let skills = $state.raw<string[]>([]);
  let formError = $state<string | null>(null);
  let busy = $state(false);

  // Résumé upload → skill extraction. The file/text is parsed server-side and discarded;
  // the returned slugs merge (union) into the skills field without wiping manual entries.
  let resumeBusy = $state(false);
  let resumeText = $state('');
  let resumeError = $state<string | null>(null);
  let resumeNote = $state<string | null>(null);
  let fileInput = $state<HTMLInputElement | null>(null);

  // Per-profile market skill-gap, keyed by profile id. Market skills are cached by the
  // sorted specialization set, so profiles sharing specializations reuse one facet fetch.
  // gapGeneration guards against out-of-order async runs: only the latest loadGaps commits.
  let gaps = $state.raw<Record<number, SkillGap>>({});
  const marketCache = new Map<string, string[]>();
  let gapGeneration = 0;

  const canSubmit = $derived(
    name.trim() !== '' && specializations.length > 0 && skills.length > 0,
  );

  // The skills typeahead: filter the loaded distribution locally (dictionary-only, so only
  // known skills are addable) and cap the visible matches. Reads the current distribution
  // at call time, so a slightly-late load is reflected on the next keystroke.
  function searchSkills(query: string): Promise<FacetOption[]> {
    const q = query.trim().toLowerCase();
    const matches = q ? skillDist.filter((o) => o.label.toLowerCase().includes(q)) : skillDist;
    return Promise.resolve(matches.slice(0, 50));
  }

  async function loadProfiles() {
    status = 'loading';
    try {
      await searchProfiles.ensureLoaded();
      status = 'ready';
    } catch {
      status = 'error';
    }
  }

  async function loadSkills() {
    try {
      const counts = await facetCounts(new URLSearchParams());
      const dist = counts.facets?.skills ?? {};
      skillDist = Object.entries(dist)
        .map(([value, count]) => ({ value, label: value, count }))
        .toSorted((a, b) => b.count - a.count || a.label.localeCompare(b.label));
    } catch {
      // best-effort: an empty distribution still lets an edited profile's skills show as
      // chips (seeded via fallbackLabel); the typeahead just has nothing to suggest.
    }
  }

  // Extract skills from a résumé (a PDF File or pasted text) and merge them into the
  // skills field as a deduplicated union, preserving anything already entered.
  async function analyzeResume(input: File | string) {
    resumeBusy = true;
    resumeError = null;
    resumeNote = null;
    try {
      const extracted = await extractResumeSkills(input);
      if (extracted.length === 0) {
        resumeNote = 'No known skills found in the résumé.';
        return;
      }
      const before = skills.length;
      skills = [...new Set([...skills, ...extracted])];
      const added = skills.length - before;
      resumeNote =
        added > 0
          ? `Added ${added} skill${added === 1 ? '' : 's'} from your résumé.`
          : 'All résumé skills were already listed.';
    } catch (err) {
      resumeError =
        err instanceof ApiError ? err.message : 'Could not read the résumé. Please try again.';
    } finally {
      resumeBusy = false;
    }
  }

  function onResumeFile(e: Event) {
    const target = e.currentTarget as HTMLInputElement;
    const file = target.files?.[0];
    target.value = ''; // clear so re-selecting the same file fires change again
    if (file) void analyzeResume(file);
  }

  // For each profile with a specialization, fetch its market skills (cached by spec set)
  // and compute the gap against the profile's skills. Best-effort: a failed facet fetch
  // simply omits that profile's gap block.
  async function loadGaps(list: SearchProfile[]) {
    const gen = ++gapGeneration;
    const next: Record<number, SkillGap> = {};
    for (const p of list) {
      if (p.specializations.length === 0) continue;
      const key = [...p.specializations].toSorted().join(',');
      let market = marketCache.get(key);
      if (!market) {
        try {
          const counts = await facetCounts(categoryParams(p.specializations));
          market = sortSkillsByCount(counts.facets?.skills ?? {});
          marketCache.set(key, market);
        } catch {
          continue;
        }
      }
      next[p.id] = computeGap(market, p.skills);
    }
    // Discard a stale run: a newer loadGaps started while we awaited, so it owns `gaps`.
    if (gen === gapGeneration) gaps = next;
  }

  // Build a /jobs link filtered to a profile's specialization(s) plus one missing skill.
  function jobsHref(specializations: string[], skill: string): string {
    const params = categoryParams(specializations);
    params.append('skills', skill);
    return `/jobs?${params}`;
  }

  // Load once the session is confirmed (the boot-time /me resolution may still be in
  // flight when the page is opened directly), mirroring ApiKeysView. Reset the per-user
  // cache on sign-out so a different user does not see the previous one's profiles.
  $effect(() => {
    if (isAuthenticated()) {
      void loadProfiles();
      void loadSkills();
    } else {
      searchProfiles.reset();
    }
  });

  // Recompute gaps whenever the profile list changes (create/edit/delete reassigns items).
  $effect(() => {
    const list = profiles;
    if (list.length) void loadGaps(list);
    else gaps = {};
  });

  function resetForm() {
    editingId = null;
    name = '';
    specializations = [];
    skills = [];
    formError = null;
  }

  function startEdit(p: SearchProfile) {
    editingId = p.id;
    name = p.name;
    specializations = [...p.specializations];
    skills = [...p.skills];
    formError = null;
  }

  // Multi-select with a cap: toggling off is always allowed; toggling on is refused past
  // MAX_SPECIALIZATIONS (with a hint) so the form matches the server's limit.
  function toggleSpecialization(value: string) {
    if (specializations.includes(value)) {
      specializations = specializations.filter((s) => s !== value);
      return;
    }
    if (specializations.length >= MAX_SPECIALIZATIONS) {
      formError = `You can pick at most ${MAX_SPECIALIZATIONS} specializations.`;
      return;
    }
    specializations = [...specializations, value];
    formError = null;
  }

  function toggleSkill(value: string) {
    skills = skills.includes(value) ? skills.filter((s) => s !== value) : [...skills, value];
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!canSubmit || busy) return;
    busy = true;
    formError = null;
    try {
      if (editingId === null) {
        await searchProfiles.create(name.trim(), specializations, skills);
      } else {
        await searchProfiles.update(editingId, { name: name.trim(), specializations, skills });
      }
      resetForm();
    } catch (err) {
      formError = err instanceof ApiError ? err.message : 'Could not save the profile. Please try again.';
    } finally {
      busy = false;
    }
  }

  async function remove(p: SearchProfile) {
    if (!window.confirm(`Delete profile “${p.name}”?`)) return;
    try {
      await searchProfiles.remove(p.id);
      if (editingId === p.id) resetForm();
    } catch {
      formError = 'Could not delete the profile. Please try again.';
    }
  }
</script>

{#if !isAuthenticated()}
  <div class="flex flex-col items-center gap-3 py-12 text-center">
    <p class="text-sm text-muted-foreground">Sign in to create search profiles.</p>
    <Button variant="primary" onclick={() => openAuthDialog()}>Sign in</Button>
  </div>
{:else}
  <div class="flex flex-col gap-6">
    <div class="flex flex-col gap-1">
      <h1 class="text-2xl font-semibold tracking-tight">Search profiles</h1>
      <p class="text-sm text-muted-foreground">
        Describe what you do — one or more specializations and your skills — and reuse it to
        find relevant work.
      </p>
    </div>

    <form onsubmit={submit} class="flex flex-col gap-4 rounded-lg border border-border p-4">
      <p class="text-sm font-medium">{editingId === null ? 'New profile' : 'Edit profile'}</p>

      <label class="flex flex-col gap-1">
        <span class="text-sm font-medium">Name</span>
        <Input bind:value={name} placeholder="e.g. Go backend" maxlength={100} class="w-full" />
      </label>

      <div class="flex flex-col gap-1.5">
        <span class="text-sm font-medium">Specializations</span>
        <SearchSelect
          options={CATEGORY_OPTIONS}
          selected={specializations}
          placeholder="Search specializations"
          onToggle={toggleSpecialization}
          clearOnSelect
        />
      </div>

      <div class="flex flex-col gap-1.5">
        <span class="text-sm font-medium">Skills</span>
        <RemoteSearchSelect
          search={searchSkills}
          selected={skills}
          placeholder="Search skills"
          onToggle={toggleSkill}
          fallbackLabel={(v) => v}
          clearOnSelect
        />
      </div>

      <div class="flex flex-col gap-1.5">
        <span class="text-sm font-medium">Add skills from your résumé</span>
        <div class="flex flex-wrap items-center gap-2">
          <input
            type="file"
            accept="application/pdf,.pdf"
            class="hidden"
            bind:this={fileInput}
            onchange={onResumeFile}
          />
          <Button
            variant="secondary"
            size="sm"
            type="button"
            onclick={() => fileInput?.click()}
            disabled={resumeBusy}
          >
            {resumeBusy ? 'Analyzing…' : 'Upload résumé (PDF)'}
          </Button>
          <span class="text-xs text-muted-foreground">extracts your skills into the field above</span>
        </div>
        <details class="text-xs">
          <summary class="cursor-pointer text-muted-foreground">or paste résumé text</summary>
          <div class="mt-2 flex flex-col gap-2">
            <textarea
              bind:value={resumeText}
              rows="4"
              placeholder="Paste your résumé here…"
              class="w-full rounded-md border border-border bg-background p-2 text-sm"
            ></textarea>
            <div>
              <Button
                variant="secondary"
                size="sm"
                type="button"
                onclick={() => analyzeResume(resumeText)}
                disabled={resumeBusy || resumeText.trim() === ''}
              >
                Extract skills
              </Button>
            </div>
          </div>
        </details>
        {#if resumeError}
          <p class="text-sm text-destructive">{resumeError}</p>
        {:else if resumeNote}
          <p class="text-xs text-muted-foreground">{resumeNote}</p>
        {/if}
      </div>

      {#if formError}
        <p class="text-sm text-destructive">{formError}</p>
      {/if}

      <div class="flex items-center gap-2">
        <Button variant="primary" type="submit" disabled={!canSubmit || busy}>
          {busy ? 'Saving…' : editingId === null ? 'Create profile' : 'Save changes'}
        </Button>
        {#if editingId !== null}
          <Button variant="ghost" type="button" onclick={resetForm}>Cancel</Button>
        {/if}
      </div>
    </form>

    {#if status === 'loading'}
      <States state="loading" />
    {:else if status === 'error'}
      <States state="error" message="Couldn't load your profiles." />
    {:else if profiles.length === 0}
      <States state="empty" message="No profiles yet. Create one above." />
    {:else}
      <ul class="flex flex-col divide-y divide-border rounded-lg border border-border">
        {#each profiles as profile (profile.id)}
          {@const gap = gaps[profile.id]}
          <li class="flex items-start justify-between gap-3 px-4 py-3">
            <div class="flex min-w-0 flex-col gap-1">
              <span class="truncate text-sm font-medium">{profile.name}</span>
              <span class="text-xs text-muted-foreground">
                {profile.specializations.map(categoryLabel).join(', ')}
              </span>
              <div class="flex flex-wrap gap-1">
                {#each profile.skills as skill (skill)}
                  <span class="rounded bg-secondary px-1.5 py-0.5 text-xs text-secondary-foreground">{skill}</span>
                {/each}
              </div>
              {#if gap && gap.total > 0}
                <div class="mt-1 flex flex-col gap-1">
                  <div class="flex items-center gap-2">
                    <span class="whitespace-nowrap text-xs text-muted-foreground">
                      Market fit {gap.coverage}/{gap.total}
                    </span>
                    <div class="h-1.5 flex-1 overflow-hidden rounded bg-secondary">
                      <div class="h-full rounded bg-primary" style="width: {(gap.coverage / gap.total) * 100}%"></div>
                    </div>
                  </div>
                  {#if gap.missing.length > 0}
                    <div class="flex flex-wrap items-center gap-1">
                      <span class="text-xs text-muted-foreground">Missing:</span>
                      {#each gap.missing as skill (skill)}
                        <a
                          href={jobsHref(profile.specializations, skill)}
                          class="rounded border border-dashed border-border px-1.5 py-0.5 text-xs text-muted-foreground hover:text-foreground"
                        >{skill}</a>
                      {/each}
                    </div>
                  {/if}
                </div>
              {/if}
            </div>
            <div class="flex shrink-0 items-center gap-1">
              <Button variant="ghost" size="sm" onclick={() => startEdit(profile)}>Edit</Button>
              <Button variant="ghost" size="sm" onclick={() => remove(profile)}>Delete</Button>
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
{/if}
