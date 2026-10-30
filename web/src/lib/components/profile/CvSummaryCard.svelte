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
  // internal/resume/owned.go), through resumeStore.saveContacts, which queues every
  // writer of that block so one card's save can't put back what another's just replaced.
  // The fields autosave like Contacts (TextAutosave), with an Undo in the status line
  // standing in for the Cancel an edit mode used to have.
  //
  // `structured` (GET /me/resume's `structured`) is non-null whenever there is anything
  // worth showing — a current CV parse, OR owned overrides alone (they survive a CV
  // delete, see internal/resume/AGENTS.md), so this stays reachable for editing even with
  // no file on record. No separate "still parsing" placeholder: a CV can sit pending
  // indefinitely (no LLM configured, a stuck job) and a permanent-looking stuck message
  // reads as broken rather than as progress.
  import { untrack } from 'svelte';
  import { resumeStore } from '#lib/resume.svelte.js';
  import { SaveStatus } from '#lib/saveStatus.svelte.js';
  import { TextAutosave } from '#lib/textAutosave.js';
  import type { ResumeStructured } from '#lib/types.js';
  import { Input } from '#lib/ui/index.js';
  import SaveStatusLine from '../SaveStatus.svelte';
  import { messages } from './CvSummaryCard.messages';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';

  let { structured }: { structured: ResumeStructured | null } = $props();

  // A current structured CV is the entry point, even if this particular parse had neither
  // field — the candidate can still add them by hand once a CV is on file.
  const showCard = $derived(structured !== null);
  const s = $derived(t(messages, locale()));
  const fieldIds = $props.id();

  type Summary = { headline: string; summary: string };

  // svelte-ignore state_referenced_locally
  let headline = $state(structured?.headline ?? '');
  // svelte-ignore state_referenced_locally
  let summary = $state(structured?.summary ?? '');
  const status = new SaveStatus();

  const read = (): Summary => ({ headline: headline.trim(), summary: summary.trim() });
  const shown = (): Summary => ({ headline: structured?.headline ?? '', summary: structured?.summary ?? '' });

  const autosave = new TextAutosave<Summary>({
    status,
    read,
    saved: shown,
    // Owned per field even when the value is "" — otherwise clearing Headline here is
    // indistinguishable, server-side, from never having set it, and the CV's own headline
    // reappears on the next read (internal/resume/owned.go).
    save: (value) => resumeStore.saveContacts({ ...value, headline_set: true, summary_set: true }),
  });

  // Follow the shown values when they change elsewhere (an Undo, a fresh CV parse), field
  // by field and reading the boxes untracked, for the reasons CandidateContactsEditor gives.
  $effect(() => {
    const now = shown();
    if (autosave.holding) return;
    const mine = untrack(read);
    if (now.headline !== mine.headline) headline = now.headline;
    if (now.summary !== mine.summary) summary = now.summary;
  });

  $effect(() => () => {
    autosave.commit(true);
  });
</script>

{#if showCard}
  <div class="flex flex-col gap-4" onfocusout={() => autosave.commit()}>
    <div class="flex items-center justify-between gap-3">
      <h2 class="text-sm font-semibold">{s.heading}</h2>
      <SaveStatusLine {status} />
    </div>

    <label class="flex flex-col gap-1 text-sm" for="{fieldIds}-headline">
      <span class="text-muted-foreground">{s.headlineLabel}</span>
      <Input
        id="{fieldIds}-headline"
        bind:value={headline}
        oninput={() => autosave.edited()}
        placeholder={s.headlinePlaceholder}
        class="w-full"
      />
    </label>
    <label class="flex flex-col gap-1 text-sm">
      <span class="text-muted-foreground">{s.heading}</span>
      <textarea
        bind:value={summary}
        oninput={() => autosave.edited()}
        rows="4"
        class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
        placeholder={s.summaryPlaceholder}
      ></textarea>
    </label>

    {#if structured?.location}
      <p class="text-xs text-muted-foreground">{format(s.cvLocation, { location: structured.location })}</p>
    {/if}
  </div>
{/if}
