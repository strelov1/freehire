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
  import { onDestroy } from 'svelte';
  import { Briefcase, FolderKanban, Plus, Sparkles } from '@lucide/svelte';
  import type { Component } from 'svelte';
  import { api } from '$lib/api';
  import { Button, ConfirmDialog, FormField, Input } from '$lib/ui';
  import CompanyPicker from '$lib/components/CompanyPicker.svelte';
  import ExperienceAssistantPanel from '$lib/components/ExperienceAssistantPanel.svelte';
  import ExperienceEmploymentCard from '$lib/components/ExperienceEmploymentCard.svelte';
  import ExperienceAchievementRow from '$lib/components/ExperienceAchievementRow.svelte';
  import States from '$lib/components/States.svelte';
  import PeriodDateInput from '$lib/components/PeriodDateInput.svelte';
  import { profileKickoff } from '$lib/assistant/presets';
  import { findFirstUnconfirmed, isUnconfirmed, sortNeedsAttentionFirst } from '$lib/experienceBank';
  import { nextResumePollDelayMs, type CvParseState } from '$lib/onboardingResumeWait';
  import type {
    ExperienceAtom,
    ExperienceBank,
    ExperienceEmployment,
    ExperienceEmploymentWithAtoms,
    PeriodDate,
  } from '$lib/types';
  import { must } from '$lib/utils';

  /** Host-supplied: profile reseeds the base CV, tailor resets the open tailored copy.
   *  The bank itself never talks to the CV store. */
  let { onBankMutated }: { onBankMutated?: () => void } = $props();

  let bank = $state<ExperienceBank | null>(null);
  let loading = $state(true);
  let error = $state('');
  let busy = $state(false);
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

  // The interviewer, docked beside the bank. `launch.id` is a remount token, not a
  // session id: aiming the chat at a different set of achievements means a new
  // conversation, and a mounted chat cannot be re-aimed (see the panel).
  let panelOpen = $state(false);
  let launch = $state({ id: 0, kickoff: profileKickoff([]) });
  // A turn in flight would be abandoned mid-answer by a relaunch, so the entries close
  // while one is running.
  let turnActive = $state(false);

  function launchInterview(ids: string[]) {
    if (turnActive) return;
    launch = { id: launch.id + 1, kickoff: profileKickoff(ids) };
    panelOpen = true;
  }

  /** A read the candidate asked for. Clears the selection, which their action consumed. */
  async function load() {
    loading = true;
    error = '';
    try {
      bank = await api.getExperience();
      selected = [];
    } catch (e) {
      error = e instanceof Error ? e.message : 'Could not load your experience.';
    } finally {
      loading = false;
    }
  }

  /**
   * A read the CONVERSATION caused. Deliberately different from `load`: it never blanks the
   * list to a spinner under an open panel, and it keeps the selection rather than clearing
   * it. A merge made in chat deletes one of two selected achievements, and throwing away
   * the other — which the candidate chose and never touched — would be the panel undoing
   * their work as a side effect of helping.
   */
  async function refreshBank() {
    try {
      const next = await api.getExperience();
      bank = next;
      const alive = new Set(
        [...next.employments.flatMap((e) => e.atoms), ...next.unplaced].map((a) => a.id),
      );
      selected = selected.filter((id) => alive.has(id));
    } catch {
      // The transcript beside it already says what happened; a failed background refetch
      // must not replace a list that is merely stale with an error.
    }
  }

  $effect(() => {
    void load();
    void loadResumeStatus();
  });

  /** Whether the candidate's résumé failed to parse — the bank's own answer to the question
   *  an empty-looking bank raises, surfaced here (and in Tailor's Experience tab, which
   *  mounts this same component) rather than only once, during onboarding. */
  let resumeParse = $state<CvParseState>('idle');
  // Supersedes an in-flight wait the same way onboarding's own `waitToken` does: a newer
  // retry, or the component going away, must stop a stale poll from overwriting a fresher
  // state.
  let resumeWaitToken = 0;

  onDestroy(() => {
    resumeWaitToken++;
  });

  async function loadResumeStatus() {
    try {
      const resume = await api.getResume();
      if (!resume.enabled || !resume.present) {
        resumeParse = 'idle';
        return;
      }
      resumeParse = resume.parse_status === 'failed' ? 'failed' : 'idle';
    } catch {
      // A failed status read is not evidence of a failed parse — leave whatever state was
      // already shown rather than flashing a banner that was never confirmed.
    }
  }

  /** Re-derives the structured résumé from what is already stored (no re-upload), then polls
   *  the same way onboarding's `waitForResumeStructure` does — the backoff table it uses
   *  (`nextResumePollDelayMs`) is shared; what happens with each state is not, since this
   *  reloads the bank instead of filling wizard fields. */
  async function retryResumeExtraction() {
    const token = ++resumeWaitToken;
    resumeParse = 'waiting';
    try {
      await api.retryResumeExtract();
    } catch {
      if (token !== resumeWaitToken) return;
      resumeParse = 'failed';
      return;
    }
    for (let attempt = 0; ; attempt++) {
      if (token !== resumeWaitToken) return;
      const delay = nextResumePollDelayMs(attempt);
      if (delay === null) {
        // Giving up is not the same as failing (see onboardingResumeWait.ts's own doc
        // comment): the retry may still land after we stop asking. Showing 'failed' here
        // would claim an outcome we never actually confirmed; the next time this view loads,
        // loadResumeStatus() reads whatever the server eventually settled on.
        resumeParse = 'idle';
        return;
      }
      // oxlint-disable-next-line no-await-in-loop -- the wait between reads is the point
      await new Promise((resolve) => setTimeout(resolve, delay));
      if (token !== resumeWaitToken) return;
      let status: string | undefined;
      try {
        // oxlint-disable-next-line no-await-in-loop -- a backoff poll is sequential by design
        const resume = await api.getResume();
        status = resume.parse_status;
      } catch {
        continue; // treated as still pending, same as onboarding's own read failure
      }
      if (token !== resumeWaitToken) return;
      if (status === 'pending') continue;
      if (status === 'failed') {
        resumeParse = 'failed';
        return;
      }
      resumeParse = 'idle';
      await refreshBank();
      return;
    }
  }

  const allAtoms = $derived(
    bank ? [...bank.employments.flatMap((e) => e.atoms), ...bank.unplaced] : [],
  );
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
    busy = true;
    try {
      await api.updateExperienceEmployment(employment.id, body);
      await load();
      onBankMutated?.();
      return true;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Could not update.';
      return false;
    } finally {
      busy = false;
    }
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
    busy = true;
    try {
      await api.deleteExperienceEmployment(employment.id);
      await load();
      onBankMutated?.();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Could not remove that entry.';
    } finally {
      busy = false;
    }
  }

  async function createProject() {
    if (busy || !projName.trim()) return;
    busy = true;
    try {
      await api.createExperienceEmployment({
        kind: 'project',
        name: projName.trim(),
        link: projLink.trim() || undefined,
        start: projStart,
        end: projEnd,
      });
      addingProject = false;
      projName = '';
      projLink = '';
      projStart = undefined;
      projEnd = undefined;
      await load();
      onBankMutated?.();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Could not create project.';
    } finally {
      busy = false;
    }
  }

  async function createJob() {
    if (busy || !jobCompany.trim()) return;
    busy = true;
    try {
      await api.createExperienceEmployment({
        kind: 'job',
        company: jobCompany.trim(),
        role: jobRole.trim() || undefined,
        location: jobLocation.trim() || undefined,
        start: jobStart,
        end: jobCurrent ? undefined : jobEnd,
        current: jobCurrent,
      });
      addingJob = false;
      jobCompany = '';
      jobRole = '';
      jobLocation = '';
      jobStart = undefined;
      jobEnd = undefined;
      jobCurrent = false;
      await load();
      onBankMutated?.();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Could not create experience.';
    } finally {
      busy = false;
    }
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
    busy = true;
    try {
      // List-only flags must not round-trip; send only writable fields.
      await api.updateExperienceAtom(atom.id, {
        claim,
        context: context || undefined,
        metrics: parseMetrics(metricsRaw),
        skills: atom.skills,
        employment_id: atom.employment_id,
      });
      await load();
      onBankMutated?.();
      return true;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Could not save that change.';
      return false;
    } finally {
      busy = false;
    }
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
    busy = true;
    try {
      await api.mergeExperienceAtoms([must(selected[0]), must(selected[1])]);
      await load();
      onBankMutated?.();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Could not merge those achievements.';
    } finally {
      busy = false;
    }
  }

  /** Confirms an unconfirmed atom without opening the edit field: re-submits its own claim
   *  unchanged, which the server re-stamps to `manual` provenance the same as any edit does. */
  async function confirmAtom(atom: ExperienceAtom) {
    if (busy) return;
    busy = true;
    try {
      await api.updateExperienceAtom(atom.id, {
        claim: atom.claim,
        context: atom.context,
        metrics: atom.metrics,
        skills: atom.skills,
        employment_id: atom.employment_id,
      });
      await load();
      onBankMutated?.();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Could not confirm that achievement.';
    } finally {
      busy = false;
    }
  }

  /** Create a project employment and attach this unplaced achievement to it. Returns
   *  whether it succeeded, so the row knows whether to leave the promote form. */
  async function savePromoteToProject(
    atom: ExperienceAtom,
    name: string,
    link: string,
  ): Promise<boolean> {
    if (busy || !name.trim()) return false;
    busy = true;
    try {
      const created = await api.createExperienceEmployment({
        kind: 'project',
        name: name.trim(),
        link: link.trim() || undefined,
      });
      await api.updateExperienceAtom(atom.id, {
        claim: atom.claim,
        context: atom.context,
        metrics: atom.metrics,
        skills: atom.skills,
        employment_id: created.id,
      });
      await load();
      onBankMutated?.();
      return true;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Could not save as project.';
      return false;
    } finally {
      busy = false;
    }
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
    busy = true;
    try {
      await api.deleteExperienceAtom(atom.id);
      await load();
      onBankMutated?.();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Could not remove that achievement.';
    } finally {
      busy = false;
    }
  }
</script>

<!-- The panel is fixed to the viewport and the account shell steps aside for it, so it is
     not laid out here despite being owned here — the bank keeps the full column it had
     rather than splitting it with the conversation about it. -->
<ExperienceAssistantPanel
  open={panelOpen}
  {launch}
  onClose={() => (panelOpen = false)}
  onBankChanged={refreshBank}
  onTurnStateChange={(active) => (turnActive = active)}
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
    {#if resumeParse === 'failed'}
      <!-- Same wording the onboarding wizard already uses for this exact state
           (ProfileLinksFields.svelte) — this is the same signal (parse_status), finally
           surfaced where a candidate actually manages experience, not only once at
           sign-up. -->
      <div class="mb-4 flex flex-wrap items-center justify-between gap-3 rounded-lg bg-warning/5 px-4 py-3 text-sm">
        <p>Couldn't read your résumé — that may be why this looks empty.</p>
        <Button size="sm" variant="outline" onclick={() => void retryResumeExtraction()}>
          Try again
        </Button>
      </div>
    {:else if resumeParse === 'waiting'}
      <div class="mb-4 rounded-lg bg-muted/40 px-4 py-3 text-sm text-muted-foreground">
        Reading your résumé again…
      </div>
    {/if}
    {#if loading}
      <States state="loading" />
    {:else if error}
      <States state="error" message={error} />
    {:else if !bank || (bank.employments.length === 0 && bank.unplaced.length === 0)}
      <!-- An empty bank is the one that most needs filling, so this state carries the same
           way in as a full one rather than a sentence and a dead end. -->
      <div class="flex flex-col items-center gap-4 py-12 text-center">
        <p class="max-w-prose text-sm text-muted-foreground">
          Nothing recorded yet. Upload a CV, or talk it through with the assistant — whatever
          you confirm is kept here and reused in every CV you build.
        </p>
        <div class="flex flex-col items-center gap-1.5">
          {@render interviewEntry()}
        </div>
      </div>
    {:else}
      <div class="flex flex-col gap-6">
        <!-- The header states what this page is FOR, because "experience bank" means nothing
             to someone meeting it for the first time. -->
        <div class="flex flex-wrap items-start justify-between gap-x-6 gap-y-3">
          <p class="max-w-prose flex-1 text-sm text-muted-foreground">
            {totalAtoms} achievement{totalAtoms === 1 ? '' : 's'} on record. These are what your
            tailored CVs are built from — edit or remove anything that is not right.
          </p>
          <!-- Capped, and right-aligned only once it shares the row: the example is two lines
               of hint, and letting it set the row's width pushed the whole action under the
               paragraph. Narrow, it takes its own line and follows the text's edge.
               `basis-full` below `sm` is what forces that own line — `flex-wrap` alone could not,
               because the paragraph beside it is `flex-1` (so `flex-basis: 0`) and never claims
               enough width to push anything down. With the action also refusing to shrink, it held
               its full 16rem on a phone and crushed the paragraph to one word per line. -->
          <div
            class="flex basis-full flex-col items-start gap-1.5 text-left sm:max-w-[16rem] sm:basis-auto sm:shrink-0 sm:items-end sm:text-right"
          >
            {@render interviewEntry()}
          </div>
        </div>

        {#if needsConfirming > 0}
          <button
            type="button"
            onclick={jumpToUnconfirmed}
            class="rounded-lg bg-warning/5 px-4 py-3 text-left text-sm transition-colors hover:bg-warning/10"
          >
            <strong class="font-medium">{needsConfirming} not confirmed.</strong>
            The assistant recorded these as its own reading of something you said. They will not
            appear on any CV until you confirm them — click to jump to one, then confirm it as-is,
            edit it to change it first, or remove it.
          </button>
        {/if}

        {#if selected.length > 0}
          <div
            class="sticky top-14 z-30 flex flex-wrap items-center gap-2 rounded-lg border border-border bg-background/95 px-3 py-2 shadow-sm backdrop-blur"
            role="toolbar"
            aria-label="Selected achievements"
          >
            <span class="text-sm text-muted-foreground">
              {selected.length} selected
            </span>
            <Button size="sm" disabled={!mergeReady || busy} onclick={requestMerge}>
              Merge
            </Button>
            <Button
              size="sm"
              variant="secondary"
              disabled={turnActive}
              onclick={() => launchInterview(selected)}
            >
              <Sparkles class="size-3.5" />Tailor with assistant
            </Button>
            <Button size="sm" variant="ghost" onclick={() => (selected = [])}>
              Clear
            </Button>
            {#if selected.length === 2 && !mergeReady}
              <span class="basis-full text-xs text-muted-foreground">
                Merge needs two achievements from the same place (or both untied).
              </span>
            {/if}
          </div>
        {/if}

        <div class="flex flex-col gap-4">
          {@render sectionHeader(Briefcase, 'Work history', 'Add experience', () => {
            addingJob = true;
            jobCompany = '';
            jobRole = '';
            jobLocation = '';
            jobStart = undefined;
            jobEnd = undefined;
            jobCurrent = false;
          })}

          {#if addingJob}
            <div class="flex flex-col gap-2 rounded-lg bg-muted/40 p-3">
              <p class="text-sm font-medium">New experience</p>
              <FormField label="Company">
                {#snippet children({ id, describedBy })}
                  <CompanyPicker {id} aria-describedby={describedBy} bind:value={jobCompany} />
                {/snippet}
              </FormField>
              <div class="flex gap-2">
                <FormField label="Role" class="flex-1">
                  {#snippet children({ id, describedBy })}
                    <Input {id} aria-describedby={describedBy} bind:value={jobRole} />
                  {/snippet}
                </FormField>
                <FormField label="Location" class="flex-1">
                  {#snippet children({ id, describedBy })}
                    <Input {id} aria-describedby={describedBy} bind:value={jobLocation} />
                  {/snippet}
                </FormField>
              </div>
              <div class="flex gap-2">
                <PeriodDateInput bind:value={jobStart} placeholder="Start" />
                {#if !jobCurrent}
                  <PeriodDateInput bind:value={jobEnd} placeholder="End" />
                {/if}
              </div>
              <label class="flex items-center gap-2 text-sm">
                <input type="checkbox" bind:checked={jobCurrent} class="h-4 w-4" />
                <span class="text-muted-foreground">I currently work here</span>
              </label>
              <div class="flex gap-2">
                <Button size="sm" disabled={busy || !jobCompany.trim()} onclick={createJob}>Save</Button>
                <Button size="sm" variant="ghost" onclick={() => (addingJob = false)}>Cancel</Button>
              </div>
            </div>
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
          {@render sectionHeader(FolderKanban, 'Projects', 'Add project', () => {
            addingProject = true;
            projName = '';
            projLink = '';
            projStart = undefined;
            projEnd = undefined;
          })}

          {#if addingProject}
            <div class="flex flex-col gap-2 rounded-lg bg-muted/40 p-3">
              <p class="text-sm font-medium">New project</p>
              <FormField label="Name">
                {#snippet children({ id, describedBy })}
                  <Input {id} aria-describedby={describedBy} bind:value={projName} />
                {/snippet}
              </FormField>
              <FormField label="Link" hint="Optional">
                {#snippet children({ id, describedBy })}
                  <Input {id} aria-describedby={describedBy} bind:value={projLink} placeholder="https://…" />
                {/snippet}
              </FormField>
              <div class="flex gap-2">
                <PeriodDateInput bind:value={projStart} placeholder="Start" />
                <PeriodDateInput bind:value={projEnd} placeholder="End" />
              </div>
              <div class="flex gap-2">
                <Button size="sm" disabled={busy || !projName.trim()} onclick={createProject}>Save</Button>
                <Button size="sm" variant="ghost" onclick={() => (addingProject = false)}>Cancel</Button>
              </div>
            </div>
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
              <h3 class="text-sm font-semibold text-foreground">Not tied to a role</h3>
              <p class="text-xs text-muted-foreground">
                Achievements from chat with no job or project yet. Save one as a project when it belongs to portfolio work.
              </p>
            </div>
            <ul class="flex flex-col gap-1.5">
              {#each sortNeedsAttentionFirst(bank.unplaced) as atom (atom.id)}
                <ExperienceAchievementRow
                  {atom}
                  selected={selected.includes(atom.id)}
                  {busy}
                  {turnActive}
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
    {turnActive}
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

{#snippet sectionHeader(icon: Component<{ class?: string }>, title: string, addLabel: string, onAdd: () => void)}
  {@const Icon = icon}
  <div class="flex items-center justify-between gap-2">
    <h2 class="flex items-center gap-2 text-base font-semibold text-foreground">
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
  <Button size="sm" variant="secondary" disabled={turnActive} onclick={() => launchInterview([])}>
    <Sparkles class="size-3.5" />Tailor your experience
  </Button>
{/snippet}
