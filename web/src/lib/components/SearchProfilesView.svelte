<script lang="ts">
  import { ApiError, facetCounts } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { CATEGORY_OPTIONS, categoryLabel, type FacetOption } from '$lib/facets';
  import { searchProfiles } from '$lib/searchProfiles.svelte';
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
