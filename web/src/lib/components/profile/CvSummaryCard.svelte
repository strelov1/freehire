<script lang="ts">
  // The flat part of the semantic body the candidate can own: headline and summary. The
  // CV-stated location is shown read-only here (a different fact from the job-search
  // "where you're based" preference on the Location view — see
  // internal/resume/AGENTS.md's three-layer table) — edit it in Contacts, which owns
  // identity. Languages, Certifications, and Education itself live on the Education view
  // instead (EducationCard) — grouped there because that's where a candidate looks for
  // them.
  //
  // Backed by the same owned-overlay PUT /me/resume/contacts as the Contacts view (see
  // internal/resume/owned.go) — a PUT replaces the whole block, so saving here spreads
  // the current `contacts` object first and only overrides the fields this card owns,
  // exactly as CandidateContactsEditor does for identity fields.
  //
  // `structured` (GET /me/resume's `structured`) is non-null whenever there is anything
  // worth showing — a current CV parse, OR owned overrides alone (they survive a CV
  // delete, see internal/resume/AGENTS.md), so this stays reachable for editing even with
  // no file on record. No separate "still parsing" placeholder: a CV can sit pending
  // indefinitely (no LLM configured, a stuck job) and a permanent-looking stuck message
  // reads as broken rather than as progress.
  import { Pencil } from '@lucide/svelte';
  import { api } from '$lib/api';
  import type { CandidateContacts, ResumeStructured } from '$lib/types';
  import { Button, Input } from '$lib/ui';

  let {
    structured,
    contacts = null,
    onSaved,
  }: {
    structured: ResumeStructured | null;
    contacts?: CandidateContacts | null;
    onSaved?: () => void;
  } = $props();

  // A current structured CV is the entry point, even if this particular parse had neither
  // field — the candidate can still add them by hand once a CV is on file.
  const showCard = $derived(structured !== null);

  let editing = $state(false);
  let headline = $state('');
  let summary = $state('');
  let busy = $state(false);
  let error = $state<string | null>(null);

  function startEdit() {
    headline = structured?.headline ?? '';
    summary = structured?.summary ?? '';
    error = null;
    editing = true;
  }

  function cancelEdit() {
    editing = false;
    error = null;
  }

  async function save() {
    busy = true;
    error = null;
    try {
      await api.putResumeContacts({
        ...contacts,
        headline: headline.trim(),
        summary: summary.trim(),
        // Owned per field even when the trimmed value above is "" — otherwise clearing
        // Headline here is indistinguishable, server-side, from never having set it, and
        // the CV's own headline reappears on the next read (internal/resume/owned.go).
        headline_set: true,
        summary_set: true,
      });
      editing = false;
      onSaved?.();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Could not save.';
    } finally {
      busy = false;
    }
  }
</script>

{#if showCard}
  <div class="flex flex-col gap-4">
    <div class="flex items-center justify-between">
      <h2 class="text-sm font-semibold">Summary</h2>
      {#if !editing}
        <Button size="sm" variant="ghost" class="text-muted-foreground" onclick={startEdit}>
          <Pencil class="size-3.5" />Edit
        </Button>
      {/if}
    </div>

    {#if editing}
      <label class="flex flex-col gap-1 text-sm">
        <span class="text-muted-foreground">Headline</span>
        <Input bind:value={headline} placeholder="e.g. Staff Backend Engineer" class="w-full" />
      </label>
      <label class="flex flex-col gap-1 text-sm">
        <span class="text-muted-foreground">Summary</span>
        <textarea
          bind:value={summary}
          rows="4"
          class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
          placeholder="A short professional summary…"
        ></textarea>
      </label>
      <div class="flex flex-wrap items-center gap-2">
        <Button size="sm" variant="primary" disabled={busy} onclick={save}>Save</Button>
        <Button size="sm" variant="ghost" class="text-muted-foreground" disabled={busy} onclick={cancelEdit}>
          Cancel
        </Button>
      </div>
      {#if error}
        <p class="text-sm text-destructive">{error}</p>
      {/if}
    {:else}
      {#if structured?.headline}
        <p class="text-sm font-medium">{structured.headline}</p>
      {/if}

      {#if structured?.summary}
        <p class="text-sm leading-relaxed">{structured.summary}</p>
      {/if}

      {#if structured?.location}
        <p class="text-xs text-muted-foreground">As stated on your CV: {structured.location}</p>
      {/if}

      {#if !structured?.headline && !structured?.summary}
        <p class="text-sm text-muted-foreground">Nothing here yet — add a headline or summary.</p>
      {/if}
    {/if}
  </div>
{/if}
