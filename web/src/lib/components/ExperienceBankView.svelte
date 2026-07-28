<script lang="ts">
  /**
   * The experience bank as its owner sees it.
   *
   * This view is the reason the bank is allowed to be written by an assistant at all. A
   * system that records claims about a person and gives them no way to see or remove those
   * claims is a trust problem before it is a compliance one — so provenance is shown on
   * every entry, and the assistant's own readings are surfaced first rather than buried.
   */
  import { Sparkles, Trash2, Pencil, Check, X } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { Button } from '$lib/ui';
  import States from '$lib/components/States.svelte';
  import type { ExperienceAtom, ExperienceBank, ExperienceProvenance } from '$lib/types';

  let bank = $state<ExperienceBank | null>(null);
  let loading = $state(true);
  let error = $state('');
  let editing = $state<string | null>(null);
  let draft = $state('');
  let busy = $state(false);

  /** How each provenance reads to the person it describes. The wording matters: the point
   *  is not to expose an enum but to tell them who said it. */
  const provenanceLabel: Record<ExperienceProvenance, string> = {
    cv_import: 'From your CV',
    stated_in_chat: 'You told the assistant',
    manual: 'You wrote this',
    agent_inferred: 'The assistant’s reading — not yet confirmed',
  };

  const unconfirmed = (a: ExperienceAtom) => a.provenance === 'agent_inferred';

  async function load() {
    loading = true;
    error = '';
    try {
      bank = await api.getExperience();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Could not load your experience.';
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void load();
  });

  const totalAtoms = $derived(
    bank ? bank.employments.reduce((n, e) => n + e.atoms.length, 0) + bank.unplaced.length : 0,
  );
  const needsConfirming = $derived(
    bank
      ? [...bank.employments.flatMap((e) => e.atoms), ...bank.unplaced].filter(unconfirmed).length
      : 0,
  );

  function startEdit(atom: ExperienceAtom) {
    editing = atom.id;
    draft = atom.claim;
  }

  /** Saving an edit re-stamps the achievement as the owner's own statement, which is how
   *  something the assistant inferred becomes usable on a CV. The copy says so. */
  async function saveEdit(atom: ExperienceAtom) {
    const claim = draft.trim();
    if (!claim || busy) return;
    busy = true;
    try {
      await api.updateExperienceAtom(atom.id, { ...atom, claim });
      editing = null;
      await load();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Could not save that change.';
    } finally {
      busy = false;
    }
  }

  async function removeAtom(atom: ExperienceAtom) {
    if (busy) return;
    if (!confirm(`Remove “${atom.claim}” from your experience?`)) return;
    busy = true;
    try {
      await api.deleteExperienceAtom(atom.id);
      await load();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Could not remove that achievement.';
    } finally {
      busy = false;
    }
  }
</script>

{#if loading}
  <States state="loading" />
{:else if error}
  <States state="error" message={error} />
{:else if !bank || (bank.employments.length === 0 && bank.unplaced.length === 0)}
  <States
    state="empty"
    message="Nothing recorded yet. Upload a CV, or talk it through with the assistant — whatever you confirm is kept here and reused in every CV you build."
  />
{:else}
  <div class="flex flex-col gap-6">
    <!-- The header states what this page is FOR, because "experience bank" means nothing
         to someone meeting it for the first time. -->
    <div class="flex flex-wrap items-baseline justify-between gap-3">
      <p class="text-sm text-muted-foreground">
        {totalAtoms} achievement{totalAtoms === 1 ? '' : 's'} on record. These are what your
        tailored CVs are built from — edit or remove anything that is not right.
      </p>
      <Button href="/my/assistant?preset=profile" size="sm" variant="secondary">
        <Sparkles class="size-4" />
        Add more with the assistant
      </Button>
    </div>

    {#if needsConfirming > 0}
      <div class="rounded-lg border border-amber-500/40 bg-amber-500/5 px-4 py-3 text-sm">
        <strong class="font-medium">{needsConfirming} not confirmed.</strong>
        The assistant recorded these as its own reading of something you said. They will not
        appear on any CV until you confirm them — edit one to make it yours, or remove it.
      </div>
    {/if}

    {#each bank.employments as employment (employment.id)}
      <section class="flex flex-col gap-2">
        <header class="flex flex-wrap items-baseline gap-x-2">
          <h3 class="text-sm font-semibold text-foreground">
            {employment.role || employment.company}
          </h3>
          {#if employment.role && employment.company}
            <span class="text-sm text-muted-foreground">{employment.company}</span>
          {/if}
          {#if employment.start || employment.end}
            <span class="text-xs text-muted-foreground">
              {employment.start}{employment.end ? ` – ${employment.end}` : ''}
            </span>
          {/if}
        </header>

        {#if employment.atoms.length === 0}
          <p class="text-sm text-muted-foreground">
            Nothing recorded for this role yet — the assistant can help you fill it in.
          </p>
        {:else}
          <ul class="flex flex-col gap-1.5">
            {#each employment.atoms as atom (atom.id)}
              {@render achievement(atom)}
            {/each}
          </ul>
        {/if}
      </section>
    {/each}

    {#if bank.unplaced.length > 0}
      <section class="flex flex-col gap-2">
        <h3 class="text-sm font-semibold text-foreground">Not tied to a role</h3>
        <ul class="flex flex-col gap-1.5">
          {#each bank.unplaced as atom (atom.id)}
            {@render achievement(atom)}
          {/each}
        </ul>
      </section>
    {/if}
  </div>
{/if}

{#snippet achievement(atom: ExperienceAtom)}
  <li
    class="group rounded-md border px-3 py-2 {unconfirmed(atom)
      ? 'border-amber-500/40 bg-amber-500/5'
      : 'border-border'}"
  >
    {#if editing === atom.id}
      <div class="flex flex-col gap-2">
        <textarea
          bind:value={draft}
          rows="2"
          class="w-full resize-y rounded border border-border bg-background px-2 py-1 text-sm"
          aria-label="Achievement"
        ></textarea>
        <div class="flex items-center gap-2">
          <Button size="sm" onclick={() => saveEdit(atom)} disabled={busy || !draft.trim()}>
            <Check class="size-4" />
            Save — this makes it yours
          </Button>
          <Button size="sm" variant="ghost" onclick={() => (editing = null)}>
            <X class="size-4" />
            Cancel
          </Button>
        </div>
      </div>
    {:else}
      <div class="flex items-start justify-between gap-3">
        <div class="min-w-0">
          <p class="text-sm text-foreground">{atom.claim}</p>
          <p class="mt-0.5 text-xs text-muted-foreground">
            {provenanceLabel[atom.provenance]}
            {#if atom.skills?.length}
              · {atom.skills.join(', ')}
            {/if}
          </p>
        </div>
        <div class="flex shrink-0 gap-1 opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100">
          <Button
            size="icon"
            variant="ghost"
            onclick={() => startEdit(atom)}
            aria-label="Edit achievement"
          >
            <Pencil class="size-4" />
          </Button>
          <Button
            size="icon"
            variant="ghost"
            onclick={() => removeAtom(atom)}
            aria-label="Remove achievement"
            class="text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
          >
            <Trash2 class="size-4" />
          </Button>
        </div>
      </div>
    {/if}
  </li>
{/snippet}
