<script lang="ts">
  /**
   * One employment (job or project): logo/header/summary/stack, its own edit-in-place
   * form, and its achievements collapsed behind a count by default — expanding is local,
   * in-place state, never a route change (specs/experience-bank).
   */
  import { tick } from 'svelte';
  import { ChevronRight, Trash2, Pencil } from '@lucide/svelte';
  import { Button, Chip, EntityLogo, FormField, Input } from '#lib/ui/index.js';
  import CompanyPicker from '#lib/components/CompanyPicker.svelte';
  import ExperienceAchievementRow from '#lib/components/ExperienceAchievementRow.svelte';
  import SkillIcon from '#lib/components/SkillIcon.svelte';
  import PeriodDateInput from '#lib/components/PeriodDateInput.svelte';
  import { companyLogoUrl } from '#lib/logo.js';
  import { sortNeedsAttentionFirst } from '#lib/experienceBank.js';
  import { formatPeriodRange } from '#lib/periodDate.js';
  import type {
    ExperienceAtom,
    ExperienceEmployment,
    ExperienceEmploymentWithAtoms,
    PeriodDate,
  } from '#lib/types.js';

  let {
    employment,
    selectedIds,
    busy,
    turnActive,
    forceExpanded = false,
    scrollToAtomId,
    onToggleSelect,
    onConfirmAtom,
    onSaveAtomEdit,
    onSavePromote,
    onRemoveAtom,
    onSaveEmployment,
    onRemoveEmployment,
  }: {
    employment: ExperienceEmploymentWithAtoms;
    selectedIds: string[];
    busy: boolean;
    turnActive: boolean;
    forceExpanded?: boolean;
    scrollToAtomId?: string;
    onToggleSelect: (id: string) => void;
    onConfirmAtom: (atom: ExperienceAtom) => void;
    onSaveAtomEdit: (
      atom: ExperienceAtom,
      claim: string,
      context: string,
      metricsRaw: string,
    ) => Promise<boolean>;
    onSavePromote: (atom: ExperienceAtom, name: string, link: string) => Promise<boolean>;
    onRemoveAtom: (atom: ExperienceAtom) => void;
    onSaveEmployment: (employment: ExperienceEmployment, body: Partial<ExperienceEmployment>) => Promise<boolean>;
    onRemoveEmployment: (employment: ExperienceEmployment) => void;
  } = $props();

  const placeLabel = $derived(
    employment.kind === 'project'
      ? employment.name || employment.role
      : employment.role || employment.company,
  );
  const placeSecondary = $derived.by(() => {
    if (employment.kind === 'project') return employment.link;
    return employment.role && employment.company ? employment.company : '';
  });

  let expanded = $state(false);

  // Reads `scrollToAtomId` too, not just `forceExpanded`: re-clicking the unconfirmed
  // banner for a different atom in this SAME employment changes the id but not the
  // (already-true) boolean, and a manually-collapsed card would otherwise never
  // re-expand for that second click.
  $effect(() => {
    if (forceExpanded && scrollToAtomId) expanded = true;
  });

  let isEditing = $state(false);
  let empName = $state('');
  let empRole = $state('');
  let empLocation = $state('');
  let empSummary = $state('');
  let empStack = $state('');
  let empLink = $state('');
  let empStart = $state<PeriodDate | undefined>(undefined);
  let empEnd = $state<PeriodDate | undefined>(undefined);
  let empCurrent = $state(false);

  function startEdit() {
    empName =
      employment.kind === 'project'
        ? employment.name || ''
        : employment.company || employment.role || '';
    empRole = employment.role || '';
    empLocation = employment.location || '';
    empSummary = employment.summary || '';
    empStack = (employment.stack ?? []).join(', ');
    empLink = employment.link || '';
    empStart = employment.start;
    empEnd = employment.end;
    empCurrent = employment.current ?? false;
    isEditing = true;
    void focusIn('input, textarea');
  }

  let cardEl = $state<HTMLDivElement>();

  // The form replaces the header's buttons and the header replaces the form, so the
  // focused element leaves the page both ways: focus follows it in, and back to Edit.
  async function focusIn(selector: string) {
    await tick();
    cardEl?.querySelector<HTMLElement>(`header ${selector}`)?.focus();
  }

  function closeEdit() {
    isEditing = false;
    void focusIn('[data-row-action="edit"]');
  }

  function parseStack(raw: string): string[] | undefined {
    const items = raw
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    return items.length ? items : undefined;
  }

  async function saveEdit() {
    if (busy) return;
    const body: Partial<ExperienceEmployment> = {
      kind: employment.kind,
      start: empStart,
      end: empEnd,
      link: empLink.trim() || undefined,
      summary: empSummary.trim() || undefined,
      stack: parseStack(empStack),
    };
    if (employment.kind === 'project') {
      body.name = empName.trim();
    } else {
      body.company = empName.trim();
      body.role = empRole.trim() || undefined;
      body.location = empLocation.trim() || undefined;
      body.end = empCurrent ? undefined : empEnd;
      body.current = empCurrent;
    }
    if (await onSaveEmployment(employment, body)) closeEdit();
  }
</script>

<div bind:this={cardEl} data-employment-id={employment.id} class="flex flex-col gap-2">
  <section class="flex gap-3">
    {#if employment.kind === 'job' && !isEditing}
      <EntityLogo
        name={employment.company || placeLabel || 'Unknown company'}
        src={companyLogoUrl(employment.company ?? '') ?? undefined}
        shape="square"
        size="md"
        class="shrink-0"
      />
    {/if}
    <div class="flex min-w-0 flex-1 flex-col gap-2">
      <header class="flex flex-wrap items-baseline gap-x-2">
        {#if isEditing}
          <div class="flex w-full flex-col gap-2">
            {#if employment.kind === 'job'}
              <FormField label="Company">
                {#snippet children({ id, describedBy })}
                  <CompanyPicker {id} aria-describedby={describedBy} bind:value={empName} />
                {/snippet}
              </FormField>
            {:else}
              <FormField label="Project name">
                {#snippet children({ id, describedBy })}
                  <Input {id} aria-describedby={describedBy} bind:value={empName} />
                {/snippet}
              </FormField>
            {/if}
            {#if employment.kind === 'job'}
              <div class="flex gap-2">
                <FormField label="Role" class="flex-1">
                  {#snippet children({ id, describedBy })}
                    <Input {id} aria-describedby={describedBy} bind:value={empRole} />
                  {/snippet}
                </FormField>
                <FormField label="Location" class="flex-1">
                  {#snippet children({ id, describedBy })}
                    <Input {id} aria-describedby={describedBy} bind:value={empLocation} />
                  {/snippet}
                </FormField>
              </div>
            {:else}
              <FormField label="Link">
                {#snippet children({ id, describedBy })}
                  <Input {id} aria-describedby={describedBy} bind:value={empLink} placeholder="https://…" />
                {/snippet}
              </FormField>
            {/if}
            <div class="flex gap-2">
              <PeriodDateInput bind:value={empStart} placeholder="Start" label="Employment start" />
              {#if !(employment.kind === 'job' && empCurrent)}
                <PeriodDateInput bind:value={empEnd} placeholder="End" label="Employment end" />
              {/if}
            </div>
            {#if employment.kind === 'job'}
              <label class="flex items-center gap-2 text-sm">
                <input type="checkbox" bind:checked={empCurrent} class="h-4 w-4" />
                <span class="text-muted-foreground">I currently work here</span>
              </label>
            {/if}
            <FormField label="Summary" hint="Optional">
              {#snippet children({ id, describedBy })}
                <textarea
                  {id}
                  aria-describedby={describedBy}
                  class="w-full resize-y rounded-lg border border-input bg-transparent px-3 py-2 text-sm focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  rows="2"
                  bind:value={empSummary}
                ></textarea>
              {/snippet}
            </FormField>
            <FormField label="Stack" hint="Comma-separated, optional">
              {#snippet children({ id, describedBy })}
                <Input {id} aria-describedby={describedBy} bind:value={empStack} />
              {/snippet}
            </FormField>
            <div class="flex gap-2">
              <Button size="sm" disabled={busy} onclick={saveEdit}>Save</Button>
              <Button size="sm" variant="ghost" onclick={closeEdit}>Cancel</Button>
            </div>
          </div>
        {:else}
          <h3 data-row-heading tabindex="-1" class="w-full text-base font-semibold text-foreground focus:outline-none">
            {placeLabel}
          </h3>
          <div class="ml-auto flex gap-1">
            <Button size="sm" variant="ghost" data-row-action="edit" onclick={startEdit}>
              <Pencil class="size-3.5" />
              Edit
            </Button>
            <Button
              size="icon"
              variant="ghost"
              onclick={() => onRemoveEmployment(employment)}
              aria-label={`Remove ${placeLabel}`}
              class="text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
            >
              <Trash2 class="size-3.5" />
            </Button>
          </div>
        {/if}
      </header>

      {#if !isEditing && (placeSecondary || employment.location || employment.start || employment.end || employment.current)}
        <p class="-mt-1 text-sm text-muted-foreground">
          {#if placeSecondary}{placeSecondary}{/if}
          {#if employment.location}· {employment.location}{/if}
          {#if employment.start || employment.end || employment.current}
            <span class="text-xs">
              · {formatPeriodRange(employment.start, employment.end, employment.current)}
            </span>
          {/if}
        </p>
      {/if}

      {#if employment.summary}
        <p class="text-sm text-foreground">{employment.summary}</p>
      {/if}
      {#if employment.stack?.length}
        <div class="flex flex-wrap gap-1.5">
          {#each employment.stack as tech (tech)}
            <Chip variant="brand">
              <SkillIcon slug={tech} class="mr-1 size-3 shrink-0" />{tech}
            </Chip>
          {/each}
        </div>
      {/if}
    </div>
  </section>

  {#if employment.atoms.length === 0}
    <p class="text-sm text-muted-foreground">
      {#if employment.kind === 'project'}
        Nothing recorded for this project yet — the assistant can help you fill it in.
      {:else}
        Nothing recorded for this role yet — the assistant can help you fill it in.
      {/if}
    </p>
  {:else}
    <button
      type="button"
      class="flex items-center gap-1 self-start text-sm text-muted-foreground transition-colors hover:text-foreground"
      onclick={() => (expanded = !expanded)}
      aria-expanded={expanded}
    >
      <ChevronRight class="size-3.5 transition-transform {expanded ? 'rotate-90' : ''}" />
      {employment.atoms.length} achievement{employment.atoms.length === 1 ? '' : 's'}
    </button>
    {#if expanded}
      <ul class="flex flex-col gap-1.5">
        {#each sortNeedsAttentionFirst(employment.atoms) as atom (atom.id)}
          <ExperienceAchievementRow
            {atom}
            selected={selectedIds.includes(atom.id)}
            {busy}
            {turnActive}
            {scrollToAtomId}
            {onToggleSelect}
            onConfirm={onConfirmAtom}
            onSaveEdit={onSaveAtomEdit}
            {onSavePromote}
            onRemove={onRemoveAtom}
          />
        {/each}
      </ul>
    {/if}
  {/if}
</div>
