<script lang="ts">
  /**
   * The experience bank as its owner sees it.
   *
   * This view is the reason the bank is allowed to be written by an assistant at all. A
   * system that records claims about a person and gives them no way to see or remove those
   * claims is a trust problem before it is a compliance one — so provenance is shown on
   * every entry, and the assistant's own readings are surfaced first rather than buried.
   *
   * State and mutation logic live here; one employment's presentation is
   * `ExperienceEmploymentCard`, one achievement's is `ExperienceAchievementRow`. Selection
   * (`selected`) stays here rather than in a card, so it survives any card's expand/collapse
   * and can span achievements from different employments (see specs/experience-bank).
   */
  import { onDestroy, tick } from 'svelte';
  import { Bot, Briefcase, FolderKanban, Plus, X } from '@lucide/svelte';
  import type { Component } from 'svelte';
  import { api } from '#lib/api.js';
  import { Alert, Button, ConfirmDialog } from '#lib/ui/index.js';
  import { ActionError } from '#lib/actionError.svelte.js';
  import { announce } from '#lib/announce.svelte.js';
  import ExperienceAssistantPanel from '#lib/components/ExperienceAssistantPanel.svelte';
  import ExperienceEmploymentCard from '#lib/components/ExperienceEmploymentCard.svelte';
  import ExperienceAchievementRow from '#lib/components/ExperienceAchievementRow.svelte';
  import States from '#lib/components/States.svelte';
  import ExperienceBankIntro from './ExperienceBankIntro.svelte';
  import CvParseBanner from './CvParseBanner.svelte';
  import UnconfirmedNotice from './UnconfirmedNotice.svelte';
  import NewJobForm from './NewJobForm.svelte';
  import NewProjectForm from './NewProjectForm.svelte';
  import { InterviewDock } from './interviewDock.svelte';
  import { findFirstUnconfirmed, isUnconfirmed, sortNeedsAttentionFirst } from '#lib/experienceBank.js';
  import { experienceBankStore } from '#lib/experienceBank.svelte.js';
  import { ResumeParseStatus } from './resumeParseStatus.svelte';
  import { atomRemovalFocus, employmentRemovalFocus } from './removalFocus';
  import type {
    ExperienceAtom,
    ExperienceBank,
    ExperienceEmployment,
    ExperienceEmploymentWithAtoms,
    PeriodDate,
  } from '#lib/types.js';
  import { errorMessage, must } from '#lib/utils.js';

  /** Host-supplied: profile reseeds the base CV, tailor resets the open tailored copy.
   *  The bank itself never talks to the CV store. */
  let { onBankMutated }: { onBankMutated?: () => void } = $props();

  let bank = $state<ExperienceBank | null>(null);

  /** Every read goes through here so the shared store gets it too, and the profile's
   *  completeness dot follows edits and assistant turns alike. The token makes the
   *  store drop a read that started before a sign-out. */
  async function readBank(): Promise<ExperienceBank> {
    const token = experienceBankStore.readToken();
    const next = await api.getExperience();
    experienceBankStore.accept(next, token);
    return next;
  }
  let loading = $state(true);
  let loadError = $state('');
  const actionError = new ActionError();
  let busy = $state(false);

  /** A mutation landed: say so, and clear a failure the candidate has now moved past. */
  function succeed(message: string) {
    actionError.clear();
    announce(message);
  }

  /** One write to the bank, holding `busy` while it runs: then the bank reloads, the host
   *  hears of it and `done` is announced; a failure shows `failed`. Returns whether it
   *  succeeded, for the callers whose row leaves its form only then. */
  async function mutate(write: () => Promise<unknown>, done: string, failed: string): Promise<boolean> {
    busy = true;
    try {
      await write();
      await reloadAfterMutation();
      onBankMutated?.();
      succeed(done);
      return true;
    } catch (e) {
      actionError.fail(e, failed);
      return false;
    } finally {
      busy = false;
    }
  }
  /** Selected achievement ids for merge / tailor. Order is click order. */
  let selected = $state<string[]>([]);
  // Deliberately separate from the job form below: "Add project" and "Add experience" are
  // independently toggleable, and sharing state between them let opening one silently
  // blank or overwrite the other's still-open, unsaved form.
  let addingProject = $state(false);
  let projName = $state('');
  let projLink = $state('');
  let projStart = $state<PeriodDate | undefined>(undefined);
  let projEnd = $state<PeriodDate | undefined>(undefined);
  let addingJob = $state(false);
  let jobCompany = $state('');
  let jobRole = $state('');
  let jobLocation = $state('');
  let jobStart = $state<PeriodDate | undefined>(undefined);
  let jobEnd = $state<PeriodDate | undefined>(undefined);
  let jobCurrent = $state(false);

  /** Where the unconfirmed-achievements banner last sent the candidate. Drives which
   *  employment card force-expands and which row scrolls into view — see
   *  `jumpToUnconfirmed` and specs/experience-bank's banner-link requirement. */
  let bannerTarget = $state<{ employmentId: string | null; atomId: string } | null>(null);

  function jumpToUnconfirmed() {
    if (!bank) return;
    bannerTarget = findFirstUnconfirmed(bank);
  }

  // The interviewer, docked beside the bank (see InterviewDock).
  const dock = new InterviewDock();

  /** A read the candidate asked for. Clears the selection, which their action consumed. */
  async function load() {
    loading = true;
    loadError = '';
    actionError.clear();
    try {
      bank = await readBank();
      selected = [];
    } catch (e) {
      loadError = errorMessage(e, 'Could not load your experience.');
    } finally {
      loading = false;
    }
  }

  /**
   * The re-read after one of the candidate's own edits. Unlike `load` it never swaps the
   * list for a spinner: that unmounted every card, which closed any other editor still open
   * and left focus on a button that no longer existed. Throws, so the mutation that called
   * it reports the failure.
   */
  async function reloadAfterMutation() {
    const next = await readBank();
    bank = next;
    const alive = new Set([...next.employments.flatMap((e) => e.atoms), ...next.unplaced].map((a) => a.id));
    selected = selected.filter((id) => alive.has(id));
  }

  /**
   * Where focus goes once a removal's confirmation dialog has closed. The dialog's own
   * focus restore aims at the Remove button that opened it, which the removal just took
   * out of the page; and nothing behind a modal dialog can take focus while it is open.
   */
  let afterDialog: (() => void) | null = null;
  $effect(() => {
    if (confirmRemoveOpen || confirmRemoveEmploymentOpen) return;
    const run = afterDialog;
    afterDialog = null;
    if (run) void tick().then(run);
  });

  /**
   * A read the CONVERSATION caused. Deliberately different from `load`: it never blanks the
   * list to a spinner under an open panel, and it keeps the selection rather than clearing
   * it. A merge made in chat deletes one of two selected achievements, and throwing away
   * the other — which the candidate chose and never touched — would be the panel undoing
   * their work as a side effect of helping.
   */
  async function refreshBank() {
    try {
      await reloadAfterMutation();
    } catch {
      // The transcript beside it already says what happened; a failed background refetch
      // must not replace a list that is merely stale with an error.
    }
  }

  $effect(() => {
    void load();
    void resumeParse.load();
  });

  // Whether the résumé failed to parse, and the retry (see ResumeParseStatus).
  const resumeParse = new ResumeParseStatus(() => refreshBank());
  onDestroy(resumeParse.stop);

  const allAtoms = $derived(bank ? [...bank.employments.flatMap((e) => e.atoms), ...bank.unplaced] : []);
  const jobs = $derived(bank?.employments.filter((e) => e.kind === 'job') ?? []);
  const projects = $derived(bank?.employments.filter((e) => e.kind === 'project') ?? []);
  const atomById = $derived(new Map(allAtoms.map((a) => [a.id, a])));

  const totalAtoms = $derived(allAtoms.length);
  const needsConfirming = $derived(allAtoms.filter(isUnconfirmed).length);

  /** Bucket key for merge validity: same employment, or both unplaced. */
  function bucketKey(atom: ExperienceAtom): string {
    return atom.employment_id ?? '__unplaced__';
  }

  const mergeReady = $derived.by(() => {
    if (selected.length !== 2) return false;
    const a = atomById.get(must(selected[0]));
    const b = atomById.get(must(selected[1]));
    return !!a && !!b && bucketKey(a) === bucketKey(b);
  });

  function toggleSelect(id: string) {
    if (selected.includes(id)) {
      selected = selected.filter((x) => x !== id);
      return;
    }
    selected = [...selected, id];
  }

  /** Saves an employment's edited fields. Returns whether it succeeded, so the card
   *  editing it knows whether to leave edit mode. */
  async function saveEmployment(
    employment: ExperienceEmployment,
    body: Partial<ExperienceEmployment>,
  ): Promise<boolean> {
    if (busy) return false;
    return mutate(() => api.updateExperienceEmployment(employment.id, body), 'Saved', 'Could not update.');
  }

  let removeEmploymentTarget = $state<ExperienceEmployment | null>(null);
  let confirmRemoveEmploymentOpen = $state(false);

  function requestRemoveEmployment(employment: ExperienceEmployment) {
    if (busy) return;
    removeEmploymentTarget = employment;
    confirmRemoveEmploymentOpen = true;
  }

  async function removeEmployment() {
    const employment = removeEmploymentTarget;
    if (!employment) return;
    const refocus = employmentRemovalFocus(employment);
    if (await mutate(() => api.deleteExperienceEmployment(employment.id), 'Removed', 'Could not remove that entry.'))
      afterDialog = refocus;
  }

  async function createProject() {
    if (busy || !projName.trim()) return;
    const input = {
      kind: 'project' as const,
      name: projName.trim(),
      link: projLink.trim() || undefined,
      start: projStart,
      end: projEnd,
    };
    const write = async () => {
      await api.createExperienceEmployment(input);
      addingProject = false;
      projName = '';
      projLink = '';
      projStart = undefined;
      projEnd = undefined;
    };
    await mutate(write, 'Project added', 'Could not create project.');
  }

  async function createJob() {
    if (busy || !jobCompany.trim()) return;
    const input = {
      kind: 'job' as const,
      company: jobCompany.trim(),
      role: jobRole.trim() || undefined,
      location: jobLocation.trim() || undefined,
      start: jobStart,
      end: jobCurrent ? undefined : jobEnd,
      current: jobCurrent,
    };
    const write = async () => {
      await api.createExperienceEmployment(input);
      addingJob = false;
      jobCompany = '';
      jobRole = '';
      jobLocation = '';
      jobStart = undefined;
      jobEnd = undefined;
      jobCurrent = false;
    };
    await mutate(write, 'Experience added', 'Could not create experience.');
  }

  function parseMetrics(raw: string): string[] {
    return raw
      .split(/[\n,]+/)
      .map((s) => s.trim())
      .filter(Boolean);
  }

  /** Saving an edit re-stamps the achievement as the owner's own statement, which is how
   *  something the assistant inferred becomes usable on a CV. Returns whether it
   *  succeeded, so the row knows whether to leave edit mode. */
  async function saveAtomEdit(
    atom: ExperienceAtom,
    claim: string,
    context: string,
    metricsRaw: string,
  ): Promise<boolean> {
    if (busy) return false;
    // List-only flags must not round-trip; send only writable fields.
    const body = {
      claim,
      context: context || undefined,
      metrics: parseMetrics(metricsRaw),
      skills: atom.skills,
      employment_id: atom.employment_id,
    };
    return mutate(() => api.updateExperienceAtom(atom.id, body), 'Saved', 'Could not save that change.');
  }

  let confirmMergeOpen = $state(false);

  const mergeTitle = $derived.by(() => {
    if (selected.length !== 2) return '';
    const a = atomById.get(must(selected[0]));
    const b = atomById.get(must(selected[1]));
    return a && b ? `Merge “${a.claim}” and “${b.claim}” into one achievement?` : '';
  });

  function requestMerge() {
    if (!mergeReady || busy || selected.length !== 2) return;
    confirmMergeOpen = true;
  }

  async function mergeSelected() {
    if (selected.length !== 2) return;
    const pair: [string, string] = [must(selected[0]), must(selected[1])];
    const write = async () => {
      await api.mergeExperienceAtoms(pair);
      // The merge consumed the selection; the survivor staying ticked would invite a
      // second merge nobody asked for.
      selected = [];
    };
    await mutate(write, 'Merged', 'Could not merge those achievements.');
  }

  /** Confirms an unconfirmed atom without opening the edit field: re-submits its own claim
   *  unchanged, which the server re-stamps to `manual` provenance the same as any edit does. */
  async function confirmAtom(atom: ExperienceAtom) {
    if (busy) return;
    const { claim, context, metrics, skills, employment_id } = atom;
    const body = { claim, context, metrics, skills, employment_id };
    await mutate(() => api.updateExperienceAtom(atom.id, body), 'Confirmed', 'Could not confirm that achievement.');
  }

  /** Create a project employment and attach this unplaced achievement to it. Returns
   *  whether it succeeded, so the row knows whether to leave the promote form. */
  async function savePromoteToProject(atom: ExperienceAtom, name: string, link: string): Promise<boolean> {
    if (busy || !name.trim()) return false;
    const write = async () => {
      const created = await api.createExperienceEmployment({
        kind: 'project',
        name: name.trim(),
        link: link.trim() || undefined,
      });
      const { claim, context, metrics, skills } = atom;
      await api.updateExperienceAtom(atom.id, { claim, context, metrics, skills, employment_id: created.id });
    };
    return mutate(write, 'Saved as project', 'Could not save as project.');
  }

  let removeTarget = $state<ExperienceAtom | null>(null);
  let confirmRemoveOpen = $state(false);

  function requestRemove(atom: ExperienceAtom) {
    if (busy) return;
    removeTarget = atom;
    confirmRemoveOpen = true;
  }

  async function removeAtom() {
    const atom = removeTarget;
    if (!atom) return;
    const refocus = atomRemovalFocus(atom);
    if (await mutate(() => api.deleteExperienceAtom(atom.id), 'Removed', 'Could not remove that achievement.'))
      afterDialog = refocus;
  }
</script>

<!-- The panel is fixed to the viewport and the account shell steps aside for it, so it is
     not laid out here despite being owned here — the bank keeps the full column it had
     rather than splitting it with the conversation about it. -->
<ExperienceAssistantPanel
  open={dock.open}
  launch={dock.launch}
  onClose={dock.close}
  onBankChanged={refreshBank}
  onTurnStateChange={(active) => (dock.turnActive = active)}
/>

<ConfirmDialog
  bind:open={confirmMergeOpen}
  title={mergeTitle}
  description="The richer one is kept; the other is removed."
  confirmLabel="Merge"
  onConfirm={mergeSelected}
/>

<ConfirmDialog
  bind:open={confirmRemoveOpen}
  title={`Remove “${removeTarget?.claim ?? ''}” from your experience?`}
  confirmLabel="Remove"
  variant="destructive"
  onConfirm={removeAtom}
/>

<ConfirmDialog
  bind:open={confirmRemoveEmploymentOpen}
  title={`Remove ${removeEmploymentTarget?.kind === 'project' ? removeEmploymentTarget?.name : removeEmploymentTarget?.company || removeEmploymentTarget?.role || 'this entry'}?`}
  description="Its achievements are removed with it."
  confirmLabel="Remove"
  variant="destructive"
  onConfirm={removeEmployment}
/>

<div class="min-w-0">
  <CvParseBanner state={resumeParse.state} onRetry={() => void resumeParse.retry()} />
  {#if loading}
    <States state="loading" />
  {:else if loadError}
    <States state="error" message={loadError} onRetry={() => void load()} />
  {:else if bank}
    <!-- An empty bank renders the same sections as a full one: it is the one that most
           needs filling, and the assistant is a paid feature, so the manual "Add" buttons
           below are the only way in that every plan has. -->
    <div class="flex flex-col gap-6">
      {#if actionError.message}
        <Alert variant="destructive" live={false} class="items-center py-2">
          <span class="flex-1">{actionError.message}</span>
          <button
            type="button"
            onclick={() => actionError.clear()}
            aria-label="Dismiss error"
            class="-m-1 inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-md hover:bg-destructive/10"
          >
            <X class="size-4" aria-hidden="true" />
          </button>
        </Alert>
      {/if}

      <ExperienceBankIntro {totalAtoms}>
        {@render interviewEntry()}
      </ExperienceBankIntro>

      {#if needsConfirming > 0}
        <UnconfirmedNotice count={needsConfirming} onJump={jumpToUnconfirmed} />
      {/if}

      {#if selected.length > 0}
        <div
          class="sticky top-14 z-30 flex flex-wrap items-center gap-2 rounded-lg border border-border bg-background/95 px-3 py-2 shadow-sm"
          role="toolbar"
          aria-label="Selected achievements"
        >
          <span class="text-sm text-muted-foreground">
            {selected.length} selected
          </span>
          <Button size="sm" disabled={!mergeReady || busy} onclick={requestMerge}>Merge</Button>
          <Button size="sm" variant="secondary" disabled={dock.turnActive} onclick={() => dock.launchFor(selected)}>
            <Bot class="size-3.5" />Tailor with assistant
          </Button>
          <Button size="sm" variant="ghost" onclick={() => (selected = [])}>Clear</Button>
          {#if selected.length === 2 && !mergeReady}
            <span class="basis-full text-xs text-muted-foreground">
              Merge needs two achievements from the same place (or both untied).
            </span>
          {/if}
        </div>
      {/if}

      <div class="flex flex-col gap-4">
        {@render sectionHeader('job', Briefcase, 'Work history', 'Add experience', () => {
          addingJob = true;
          jobCompany = '';
          jobRole = '';
          jobLocation = '';
          jobStart = undefined;
          jobEnd = undefined;
          jobCurrent = false;
        })}

        {#if addingJob}
          <NewJobForm
            bind:company={jobCompany}
            bind:role={jobRole}
            bind:location={jobLocation}
            bind:start={jobStart}
            bind:end={jobEnd}
            bind:current={jobCurrent}
            {busy}
            onSave={createJob}
            onCancel={() => (addingJob = false)}
          />
        {/if}

        <div class="flex flex-col gap-6">
          {#each jobs as employment (employment.id)}
            {@render employmentCard(employment)}
          {/each}
        </div>
        {#if jobs.length === 0 && !addingJob}
          <p class="text-sm text-muted-foreground">Nothing here yet.</p>
        {/if}
      </div>

      <div class="flex flex-col gap-4">
        {@render sectionHeader('project', FolderKanban, 'Projects', 'Add project', () => {
          addingProject = true;
          projName = '';
          projLink = '';
          projStart = undefined;
          projEnd = undefined;
        })}

        {#if addingProject}
          <NewProjectForm
            bind:name={projName}
            bind:link={projLink}
            bind:start={projStart}
            bind:end={projEnd}
            {busy}
            onSave={createProject}
            onCancel={() => (addingProject = false)}
          />
        {/if}

        <div class="flex flex-col gap-6">
          {#each projects as employment (employment.id)}
            {@render employmentCard(employment)}
          {/each}
        </div>
        {#if projects.length === 0 && !addingProject}
          <p class="text-sm text-muted-foreground">Nothing here yet.</p>
        {/if}
      </div>

      {#if bank.unplaced.length > 0}
        <section class="flex flex-col gap-2">
          <div class="flex flex-col gap-0.5">
            <h3
              id="experience-section-unplaced"
              tabindex="-1"
              class="text-sm font-semibold text-foreground focus:outline-none"
            >
              Not tied to a role
            </h3>
            <p class="text-xs text-muted-foreground">
              Achievements from chat with no job or project yet. Save one as a project when it belongs to portfolio
              work.
            </p>
          </div>
          <ul class="flex flex-col gap-1.5">
            {#each sortNeedsAttentionFirst(bank.unplaced) as atom (atom.id)}
              <ExperienceAchievementRow
                {atom}
                selected={selected.includes(atom.id)}
                {busy}
                turnActive={dock.turnActive}
                scrollToAtomId={bannerTarget?.employmentId === null ? bannerTarget.atomId : undefined}
                onToggleSelect={toggleSelect}
                onConfirm={confirmAtom}
                onSaveEdit={saveAtomEdit}
                onSavePromote={savePromoteToProject}
                onRemove={requestRemove}
              />
            {/each}
          </ul>
        </section>
      {/if}
    </div>
  {/if}
</div>

{#snippet employmentCard(employment: ExperienceEmploymentWithAtoms)}
  {@const isBannerTarget = bannerTarget?.employmentId === employment.id}
  <ExperienceEmploymentCard
    {employment}
    selectedIds={selected}
    {busy}
    turnActive={dock.turnActive}
    forceExpanded={isBannerTarget}
    scrollToAtomId={isBannerTarget ? bannerTarget?.atomId : undefined}
    onToggleSelect={toggleSelect}
    onConfirmAtom={confirmAtom}
    onSaveAtomEdit={saveAtomEdit}
    onSavePromote={savePromoteToProject}
    onRemoveAtom={requestRemove}
    onSaveEmployment={saveEmployment}
    onRemoveEmployment={requestRemoveEmployment}
  />
{/snippet}

{#snippet sectionHeader(
  kind: 'job' | 'project',
  icon: Component<{ class?: string }>,
  title: string,
  addLabel: string,
  onAdd: () => void,
)}
  {@const Icon = icon}
  <div class="flex items-center justify-between gap-2">
    <!-- Focusable from script only: where focus lands when the last entry under it goes. -->
    <h2
      id="experience-section-{kind}"
      tabindex="-1"
      class="flex items-center gap-2 text-base font-semibold text-foreground focus:outline-none"
    >
      <Icon class="size-4.5" />{title}
    </h2>
    <Button
      size="icon"
      variant="ghost"
      class="text-muted-foreground"
      disabled={busy}
      title={addLabel}
      aria-label={addLabel}
      onclick={onAdd}
    >
      <Plus class="size-4" />
    </Button>
  </div>
{/snippet}

<!-- The way into the interviewer. The label names what the owner gets, not the machine
     that produces it. The caller wraps this to set its alignment.

     It opens the panel rather than navigating: the bank is what the conversation is
     about, and leaving it to reach the agent is what this replaced. -->
{#snippet interviewEntry()}
  <Button size="sm" variant="secondary" disabled={dock.turnActive} onclick={() => dock.launchFor([])}>
    <Bot class="size-3.5" />Tailor your experience
  </Button>
{/snippet}
