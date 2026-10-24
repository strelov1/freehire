<script lang="ts">
  /**
   * One banked achievement: display / editing / promote-to-project. All mutation calls
   * stay with the caller (the bank owns loading, error handling and re-fetching) — this
   * component only reports intent and holds the transient edit/promote drafts, which are
   * inherently per-row and were never meant to be shared across achievements.
   */
  import { Trash2, Pencil, Check, X } from '@lucide/svelte';
  import { scrollBehavior } from '#lib/motion.js';
  import { Button, Chip, FormField, Input } from '#lib/ui/index.js';
  import SkillIcon from '#lib/components/SkillIcon.svelte';
  import { isUnconfirmed } from '#lib/experienceBank.js';
  import type { ExperienceAtom, ExperienceProvenance } from '#lib/types.js';

  let {
    atom,
    selected,
    busy,
    turnActive,
    scrollToAtomId,
    onToggleSelect,
    onConfirm,
    onSaveEdit,
    onSavePromote,
    onRemove,
  }: {
    atom: ExperienceAtom;
    selected: boolean;
    busy: boolean;
    turnActive: boolean;
    scrollToAtomId?: string;
    onToggleSelect: (id: string) => void;
    onConfirm: (atom: ExperienceAtom) => void;
    onSaveEdit: (
      atom: ExperienceAtom,
      claim: string,
      context: string,
      metricsRaw: string,
    ) => Promise<boolean>;
    onSavePromote: (atom: ExperienceAtom, name: string, link: string) => Promise<boolean>;
    onRemove: (atom: ExperienceAtom) => void;
  } = $props();

  /** How each provenance reads to the person it describes. The wording matters: the point
   *  is not to expose an enum but to tell them who said it. */
  const provenanceLabel: Record<ExperienceProvenance, string> = {
    cv_import: 'From your CV',
    stated_in_chat: 'You told the assistant',
    manual: 'You wrote this',
    agent_inferred: 'The assistant’s reading — not yet confirmed',
  };

  const unconfirmed = $derived(isUnconfirmed(atom));

  const rowBackground = $derived.by(() => {
    if (unconfirmed) return 'bg-warning/5';
    if (selected) return 'bg-brand/5';
    return '';
  });

  // Matches design-system Input's own styling — there is no design-system Textarea to
  // reach for instead (see design.md), so this mirrors it by hand for the three fields
  // below rather than drifting across three separately-typed class strings.
  const textareaClass =
    'w-full resize-y rounded-lg border border-input bg-transparent px-3 py-2 text-sm focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring';

  let isEditing = $state(false);
  let draftClaim = $state('');
  let draftContext = $state('');
  let draftMetrics = $state('');

  function startEdit() {
    draftClaim = atom.claim;
    draftContext = atom.context ?? '';
    draftMetrics = (atom.metrics ?? []).join('\n');
    isEditing = true;
  }

  async function saveEdit() {
    const claim = draftClaim.trim();
    if (!claim || busy) return;
    if (await onSaveEdit(atom, claim, draftContext.trim(), draftMetrics)) {
      isEditing = false;
    }
  }

  let isPromoting = $state(false);
  let promoteName = $state('');
  let promoteLink = $state('');

  function startPromote() {
    // Prefer a short name from context when the chat mentioned a project; otherwise leave blank.
    promoteName = (atom.context ?? '').trim().slice(0, 80);
    promoteLink = '';
    isPromoting = true;
  }

  async function savePromote() {
    if (busy || !promoteName.trim()) return;
    if (await onSavePromote(atom, promoteName.trim(), promoteLink.trim())) {
      isPromoting = false;
    }
  }

  let rowEl = $state<HTMLLIElement>();

  $effect(() => {
    if (scrollToAtomId === atom.id && rowEl) {
      rowEl.scrollIntoView({ behavior: scrollBehavior(), block: 'center' });
      rowEl.focus();
    }
  });
</script>

<li
  bind:this={rowEl}
  tabindex="-1"
  class="rounded-md py-2 pl-1 pr-2 {rowBackground}"
>
  {#if isEditing}
    <div class="flex flex-col gap-2">
      <FormField label="Achievement">
        {#snippet children({ id, describedBy })}
          <textarea
            {id}
            aria-describedby={describedBy}
            bind:value={draftClaim}
            rows="2"
            class={textareaClass}
          ></textarea>
        {/snippet}
      </FormField>
      <FormField label="Context" hint="Where this happened — team, product, constraint…">
        {#snippet children({ id, describedBy })}
          <textarea
            {id}
            aria-describedby={describedBy}
            bind:value={draftContext}
            rows="2"
            class={textareaClass}
          ></textarea>
        {/snippet}
      </FormField>
      <FormField label="Metrics" hint="One per line or comma-separated — e.g. 40%">
        {#snippet children({ id, describedBy })}
          <textarea
            {id}
            aria-describedby={describedBy}
            bind:value={draftMetrics}
            rows="2"
            class="{textareaClass} font-mono"
          ></textarea>
        {/snippet}
      </FormField>
      <div class="flex items-center gap-2">
        <Button size="sm" onclick={saveEdit} disabled={busy || !draftClaim.trim()}>
          <Check class="size-4" />
          Save — this makes it yours
        </Button>
        <Button size="sm" variant="ghost" onclick={() => (isEditing = false)}>
          <X class="size-4" />
          Cancel
        </Button>
      </div>
    </div>
  {:else if isPromoting}
    <div class="flex flex-col gap-2">
      <p class="text-sm text-foreground">{atom.claim}</p>
      <FormField label="Project name">
        {#snippet children({ id, describedBy })}
          <Input {id} aria-describedby={describedBy} bind:value={promoteName} placeholder="e.g. Sandrock, Git helpers" />
        {/snippet}
      </FormField>
      <FormField label="Link (optional)">
        {#snippet children({ id, describedBy })}
          <Input {id} aria-describedby={describedBy} bind:value={promoteLink} placeholder="https://" />
        {/snippet}
      </FormField>
      <div class="flex items-center gap-2">
        <Button size="sm" onclick={savePromote} disabled={busy || !promoteName.trim()}>
          <Check class="size-4" />
          Save as project
        </Button>
        <Button size="sm" variant="ghost" onclick={() => (isPromoting = false)}>
          <X class="size-4" />
          Cancel
        </Button>
      </div>
    </div>
  {:else}
    <div class="flex items-start gap-2">
      <input
        type="checkbox"
        class="mt-1 size-4 shrink-0 accent-brand"
        checked={selected}
        onchange={() => onToggleSelect(atom.id)}
        aria-label="Select achievement"
      />
      <div class="min-w-0 flex-1">
        <p class="text-sm text-foreground">{atom.claim}</p>
        {#if atom.context}
          <p class="mt-1 text-xs text-muted-foreground">{atom.context}</p>
        {/if}
        {#if atom.metrics?.length}
          <p class="mt-1 flex flex-wrap gap-1.5">
            {#each atom.metrics as metric (metric)}
              <Chip class="border-transparent font-mono">{metric}</Chip>
            {/each}
          </p>
        {/if}
        <p class="mt-1 text-xs text-muted-foreground">{provenanceLabel[atom.provenance]}</p>
        {#if atom.skills?.length}
          <p class="mt-1 flex flex-wrap gap-1.5">
            {#each atom.skills as skill (skill)}
              <Chip variant="brand">
                <SkillIcon slug={skill} class="mr-1 size-3 shrink-0" />{skill}
              </Chip>
            {/each}
          </p>
        {/if}
        {#if atom.cluster_id || atom.needs_context || atom.needs_metrics}
          <p class="mt-1 flex flex-wrap gap-1.5">
            {#if atom.cluster_id}
              <Chip class="border-transparent">Looks similar to another</Chip>
            {/if}
            {#if atom.needs_context}
              <Chip class="border-transparent">Thin on context</Chip>
            {/if}
            {#if atom.needs_metrics}
              <Chip class="border-transparent">No number yet</Chip>
            {/if}
          </p>
        {/if}
        {#if !atom.employment_id}
          <div class="mt-2">
            <Button size="sm" variant="secondary" onclick={startPromote} disabled={busy || turnActive}>
              Save as project
            </Button>
          </div>
        {/if}
      </div>
      <!-- Deliberately NOT hover-gated. This page exists so its owner can correct what
           was recorded about them; hiding the way to do that until the pointer lands on
           the right row means most people never learn it is possible, and a touch device
           never hovers at all. -->
      <div class="flex shrink-0 gap-1">
        {#if unconfirmed}
          <Button
            size="icon"
            variant="ghost"
            onclick={() => onConfirm(atom)}
            disabled={busy}
            aria-label="Confirm achievement"
            class="text-muted-foreground hover:bg-brand-muted hover:text-brand-strong"
          >
            <Check class="size-4" />
          </Button>
        {/if}
        <Button size="icon" variant="ghost" onclick={startEdit} aria-label="Edit achievement">
          <Pencil class="size-4" />
        </Button>
        <Button
          size="icon"
          variant="ghost"
          onclick={() => onRemove(atom)}
          aria-label="Remove achievement"
          class="text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
        >
          <Trash2 class="size-4" />
        </Button>
      </div>
    </div>
  {/if}
</li>
