<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import { announce } from '#lib/announce.svelte.js';
  import type { CoverLetterEvidenceView } from '#lib/cv.js';

  let { cvId, onwrite, onsaved = () => {} }: { cvId: string; onwrite: () => void; onsaved?: () => void } = $props();
  let view = $state.raw<CoverLetterEvidenceView | null>(null);
  let selected = $state<string[]>([]);
  let loading = $state(true);
  let saving = $state(false);
  let error = $state('');
  let message = $state('');
  let sequence = 0;
  let alive = true;
  const invalid = $derived(selected.filter((id) => !view?.available.some((a) => a.id === id)));
  const suggestedIDs = $derived(view?.suggested_ids ?? []);
  const suggested = $derived(view?.available.filter((a) => suggestedIDs.includes(a.id)) ?? []);
  const other = $derived(view?.available.filter((a) => !suggestedIDs.includes(a.id)) ?? []);

  async function load(id: string, mode: 'initial' | 'facts' | 'conflict' = 'initial') {
    const request = ++sequence;
    loading = true;
    error = '';
    try {
      const next = await api.getCoverLetterEvidence(id);
      if (!alive || request !== sequence || id !== cvId) return;
      view = mode === 'facts' && view ? { ...next, revision: view.revision } : next;
      if (mode === 'initial') selected = [...(next.explicit ? next.atom_ids : next.suggested_ids)];
    } catch (e) {
      if (alive && request === sequence && id === cvId)
        error = e instanceof Error ? e.message : 'Could not load your evidence.';
    } finally {
      if (alive && request === sequence && id === cvId) loading = false;
    }
  }

  function toggle(id: string) {
    selected = selected.includes(id) ? selected.filter((value) => value !== id) : [...selected, id];
    message = '';
  }

  async function save(write: boolean) {
    if (!view || saving) return;
    const id = cvId;
    const request = sequence;
    const choices = [...selected];
    saving = true;
    error = '';
    message = '';
    try {
      const saved = await api.saveCoverLetterEvidence(id, view.revision, choices);
      if (!alive || request !== sequence || id !== cvId) return;
      view = { ...view, revision: saved.revision, explicit: true, atom_ids: saved.atom_ids };
      message = 'Selection saved';
      announce(message);
      onsaved();
      if (write) onwrite();
    } catch (e) {
      if (!alive || request !== sequence || id !== cvId) return;
      if (e instanceof Error && 'status' in e && e.status === 409) {
        await load(id, 'conflict');
        if (!alive || id !== cvId) return;
        error = 'The saved selection changed in another tab. Your choices are still here. Review them and save again.';
      } else error = e instanceof Error ? e.message : 'Could not save your selection.';
      announce(error, 'assertive');
    } finally {
      if (alive && id === cvId) saving = false;
    }
  }

  $effect(() => {
    const id = cvId;
    view = null;
    selected = [];
    saving = false;
    message = '';
    untrack(() => {
      void load(id);
    });
  });
  onDestroy(() => {
    alive = false;
    sequence++;
  });
</script>

<section class="mb-4 rounded-xl border border-border bg-background p-4" aria-label="Cover-letter evidence">
  <h4 class="text-sm font-semibold">Your experience, for this letter</h4>
  <p class="mt-1 text-xs leading-relaxed text-muted-foreground">
    Choose the facts this letter may use. Your Experience Bank stays intact.
  </p>
  {#if loading}<p class="mt-3 text-sm text-muted-foreground">Loading your experience…</p>{/if}
  {#if view}
    <div class="my-3 flex items-center justify-between text-xs">
      <strong>Suggested evidence</strong><span>{selected.length} selected</span>
    </div>
    {#snippet fact(atom: CoverLetterEvidenceView['available'][number])}
      <div class="mb-2 rounded-lg border border-border p-3">
        <label class="flex cursor-pointer items-start gap-3 text-sm">
          <input
            type="checkbox"
            aria-label={atom.claim}
            class="mt-1 accent-primary"
            checked={selected.includes(atom.id)}
            disabled={saving || loading}
            onchange={() => toggle(atom.id)}
          />
          <span
            >{atom.claim}<span class="mt-1 block text-xs text-muted-foreground"
              >{atom.provenance === 'cv_import' ? 'From your CV' : 'Confirmed experience'}
              {#if atom.employment_id && view?.employment_labels?.[atom.employment_id]}
                · {view.employment_labels[atom.employment_id]}{/if}</span
            ></span
          >
        </label>
        {#if atom.context}<details class="ml-7 mt-2 text-xs text-muted-foreground">
            <summary class="cursor-pointer">Details</summary>
            <p class="mt-2 whitespace-pre-wrap">{atom.context}</p>
          </details>{/if}
      </div>
    {/snippet}
    {#each suggested as atom (atom.id)}{@render fact(atom)}{/each}
    {#if other.length}<details class="mt-3">
        <summary class="cursor-pointer text-xs font-medium">More from your experience ({other.length})</summary>
        <div class="mt-3">
          {#each other as atom (atom.id)}{@render fact(atom)}{/each}
        </div>
      </details>{/if}
    {#if !view.available.length}<p class="my-3 text-sm text-muted-foreground">
        Add or confirm an achievement in your Experience Bank to get started.
      </p>{/if}
    {#if invalid.length}
      <div class="my-3 text-xs text-destructive">
        <p>{invalid.length} selected fact(s) are no longer available to cite.</p>
        <button
          type="button"
          class="mt-1 underline"
          onclick={() => {
            selected = selected.filter((id) => !invalid.includes(id));
          }}>Remove unavailable facts</button
        >
      </div>
    {/if}
    <div class="my-4 flex gap-4 text-xs">
      <a
        class="text-primary underline"
        href={resolve('/my/profile/experience')}
        target="_blank"
        rel="noopener noreferrer">Add or edit experience ↗</a
      ><button
        type="button"
        class="text-primary underline"
        disabled={loading || saving}
        onclick={() => void load(cvId, 'facts')}>Refresh facts</button
      >
    </div>
    <div class="flex flex-wrap gap-2">
      <button
        type="button"
        class="rounded-md border border-border px-3 py-2 text-xs disabled:opacity-50"
        disabled={saving || loading || invalid.length > 0 || selected.length > 50}
        onclick={() => void save(false)}>Save selection</button
      >
      <button
        type="button"
        class="rounded-md bg-primary px-3 py-2 text-xs text-primary-foreground disabled:opacity-50"
        disabled={saving || loading || !selected.length || invalid.length > 0 || selected.length > 50}
        onclick={() => void save(true)}>{saving ? 'Saving…' : 'Write using these facts'}</button
      >
    </div>
    {#if !selected.length}<p class="mt-2 text-xs text-muted-foreground">
        Select at least one confirmed experience to write a letter.
      </p>{:else if selected.length > 50}<p class="mt-2 text-xs text-destructive">Choose at most 50 facts.</p>{/if}
  {/if}
  {#if error}<p class="mt-3 text-xs text-destructive" role="alert">{error}</p>{/if}
  {#if message}<p class="mt-2 text-xs text-muted-foreground" role="status">{message}</p>{/if}
</section>
