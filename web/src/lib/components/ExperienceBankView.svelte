<script lang="ts">
  /**
   * The experience bank as its owner sees it.
   *
   * This view is the reason the bank is allowed to be written by an assistant at all. A
   * system that records claims about a person and gives them no way to see or remove those
   * claims is a trust problem before it is a compliance one — so provenance is shown on
   * every entry, and the assistant's own readings are surfaced first rather than buried.
   */
  import { Trash2, Pencil, Check, X } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { Button, ConfirmDialog } from '$lib/ui';
  import ExperienceAssistantPanel from '$lib/components/ExperienceAssistantPanel.svelte';
  import States from '$lib/components/States.svelte';
  import { profileKickoff } from '$lib/assistant/presets';
  import type {
    ExperienceAtom,
    ExperienceBank,
    ExperienceEmployment,
    ExperienceEmploymentWithAtoms,
    ExperienceProvenance,
  } from '$lib/types';
  import { must } from '$lib/utils';

  /** Host-supplied: profile reseeds the base CV, tailor resets the open tailored copy.
   *  The bank itself never talks to the CV store. */
  let { onBankMutated }: { onBankMutated?: () => void } = $props();

  let bank = $state<ExperienceBank | null>(null);
  let loading = $state(true);
  let error = $state('');
  let editing = $state<string | null>(null);
  let draftClaim = $state('');
  let draftContext = $state('');
  let draftMetrics = $state('');
  let busy = $state(false);
  /** Selected achievement ids for merge / tailor. Order is click order. */
  let selected = $state<string[]>([]);
  let editingEmploymentId = $state<string | null>(null);
  let empName = $state('');
  let empRole = $state('');
  let empLocation = $state('');
  let empSummary = $state('');
  let empStack = $state('');
  let empLink = $state('');
  let empStart = $state('');
  let empEnd = $state('');
  // Deliberately separate from empName/empLink/empStart/empEnd above: "Add project" and
  // "Edit employment" are independently toggleable, and sharing state between them let
  // opening one silently blank or overwrite the other's still-open, unsaved form.
  let addingProject = $state(false);
  let projName = $state('');
  let projLink = $state('');
  let projStart = $state('');
  let projEnd = $state('');
  /** Unplaced achievement being promoted into a new project employment. */
  let promotingAtomId = $state<string | null>(null);
  let promoteName = $state('');
  let promoteLink = $state('');

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

  /** How each provenance reads to the person it describes. The wording matters: the point
   *  is not to expose an enum but to tell them who said it. */
  const provenanceLabel: Record<ExperienceProvenance, string> = {
    cv_import: 'From your CV',
    stated_in_chat: 'You told the assistant',
    manual: 'You wrote this',
    agent_inferred: 'The assistant’s reading — not yet confirmed',
  };

  const unconfirmed = (a: ExperienceAtom) => a.provenance === 'agent_inferred';

  /** Unconfirmed first. The banner tells the owner something needs a decision; leaving
   *  those entries wherever the server happened to return them makes that a scavenger
   *  hunt at eleven achievements and useless at two hundred. */
  const needsAttentionFirst = (atoms: ExperienceAtom[]) =>
    [...atoms].sort((a, b) => Number(unconfirmed(b)) - Number(unconfirmed(a)));

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
  });

  const allAtoms = $derived(
    bank ? [...bank.employments.flatMap((e) => e.atoms), ...bank.unplaced] : [],
  );
  const jobs = $derived(bank?.employments.filter((e) => e.kind === 'job') ?? []);
  const projects = $derived(bank?.employments.filter((e) => e.kind === 'project') ?? []);
  const atomById = $derived(new Map(allAtoms.map((a) => [a.id, a])));

  const totalAtoms = $derived(allAtoms.length);
  const needsConfirming = $derived(allAtoms.filter(unconfirmed).length);

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

  function startEdit(atom: ExperienceAtom) {
    editing = atom.id;
    draftClaim = atom.claim;
    draftContext = atom.context ?? '';
    draftMetrics = (atom.metrics ?? []).join('\n');
  }

  function startEditEmployment(employment: ExperienceEmployment) {
    editingEmploymentId = employment.id;
    empName =
      employment.kind === 'project'
        ? employment.name || ''
        : employment.company || employment.role || '';
    empRole = employment.role || '';
    empLocation = employment.location || '';
    empSummary = employment.summary || '';
    empStack = (employment.stack ?? []).join(', ');
    empLink = employment.link || '';
    empStart = employment.start || '';
    empEnd = employment.end || '';
  }

  function parseStack(raw: string): string[] | undefined {
    const items = raw
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    return items.length ? items : undefined;
  }

  async function saveEmployment(employment: ExperienceEmployment) {
    if (busy) return;
    busy = true;
    try {
      const body: Partial<ExperienceEmployment> = {
        kind: employment.kind,
        start: empStart.trim() || undefined,
        end: empEnd.trim() || undefined,
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
      }
      await api.updateExperienceEmployment(employment.id, body);
      editingEmploymentId = null;
      await load();
      onBankMutated?.();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Could not update.';
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
        start: projStart.trim() || undefined,
        end: projEnd.trim() || undefined,
      });
      addingProject = false;
      projName = '';
      projLink = '';
      projStart = '';
      projEnd = '';
      await load();
      onBankMutated?.();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Could not create project.';
    } finally {
      busy = false;
    }
  }

  function startPromoteToProject(atom: ExperienceAtom) {
    promotingAtomId = atom.id;
    // Prefer a short name from context when the chat mentioned a project; otherwise leave blank.
    promoteName = (atom.context ?? '').trim().slice(0, 80);
    promoteLink = '';
  }

  /** Create a project employment and attach this unplaced achievement to it. */
  async function savePromoteToProject(atom: ExperienceAtom) {
    if (busy || !promoteName.trim()) return;
    busy = true;
    try {
      const created = await api.createExperienceEmployment({
        kind: 'project',
        name: promoteName.trim(),
        link: promoteLink.trim() || undefined,
      });
      await api.updateExperienceAtom(atom.id, {
        claim: atom.claim,
        context: atom.context,
        metrics: atom.metrics,
        skills: atom.skills,
        employment_id: created.id,
      });
      promotingAtomId = null;
      promoteName = '';
      promoteLink = '';
      await load();
      onBankMutated?.();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Could not save as project.';
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
   *  something the assistant inferred becomes usable on a CV. The copy says so. */
  async function saveEdit(atom: ExperienceAtom) {
    const claim = draftClaim.trim();
    if (!claim || busy) return;
    busy = true;
    try {
      // List-only flags must not round-trip; send only writable fields.
      await api.updateExperienceAtom(atom.id, {
        claim,
        context: draftContext.trim() || undefined,
        metrics: parseMetrics(draftMetrics),
        skills: atom.skills,
        employment_id: atom.employment_id,
      });
      editing = null;
      await load();
      onBankMutated?.();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Could not save that change.';
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
          {@render interviewEntry('bank-empty-example')}
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
            <Button
              size="sm"
              variant="secondary"
              disabled={busy}
              onclick={() => {
                addingProject = true;
                projName = '';
                projLink = '';
                projStart = '';
                projEnd = '';
              }}
            >
              Add project
            </Button>
            {@render interviewEntry('bank-example')}
          </div>
        </div>

        {#if addingProject}
          <div class="flex flex-col gap-2 rounded-lg border border-border p-3">
            <p class="text-sm font-medium">New project</p>
            <input class="rounded-md border border-border bg-background px-3 py-2 text-sm" bind:value={projName} placeholder="Name" />
            <input class="rounded-md border border-border bg-background px-3 py-2 text-sm" bind:value={projLink} placeholder="https://…" />
            <div class="flex gap-2">
              <input class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm" bind:value={projStart} placeholder="Start" />
              <input class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm" bind:value={projEnd} placeholder="End" />
            </div>
            <div class="flex gap-2">
              <Button size="sm" disabled={busy || !projName.trim()} onclick={createProject}>Save</Button>
              <Button size="sm" variant="ghost" onclick={() => (addingProject = false)}>Cancel</Button>
            </div>
          </div>
        {/if}

        {#if needsConfirming > 0}
          <div class="rounded-lg border border-warning/40 bg-warning/5 px-4 py-3 text-sm">
            <strong class="font-medium">{needsConfirming} not confirmed.</strong>
            The assistant recorded these as its own reading of something you said. They will not
            appear on any CV until you confirm them — click the check to confirm as-is, edit one to
            change it first, or remove it.
          </div>
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
              Tailor with assistant
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

        {#if jobs.length > 0}
          <div class="flex flex-col gap-6">
            <h2 class="text-sm font-semibold text-foreground">Work history</h2>
            {#each jobs as employment (employment.id)}
              {@render employmentSection(employment)}
            {/each}
          </div>
        {/if}

        {#if projects.length > 0}
          <div class="flex flex-col gap-6">
            <h2 class="text-sm font-semibold text-foreground">Projects</h2>
            {#each projects as employment (employment.id)}
              {@render employmentSection(employment)}
            {/each}
          </div>
        {/if}

        {#if bank.unplaced.length > 0}
          <section class="flex flex-col gap-2">
            <div class="flex flex-col gap-0.5">
              <h3 class="text-sm font-semibold text-foreground">Not tied to a role</h3>
              <p class="text-xs text-muted-foreground">
                Achievements from chat with no job or project yet. Save one as a project when it belongs to portfolio work.
              </p>
            </div>
            <ul class="flex flex-col gap-1.5">
              {#each needsAttentionFirst(bank.unplaced) as atom (atom.id)}
                {@render achievement(atom)}
              {/each}
            </ul>
          </section>
        {/if}
      </div>
    {/if}
</div>

{#snippet employmentSection(employment: ExperienceEmploymentWithAtoms)}
  {@const placeLabel =
    employment.kind === 'project'
      ? employment.name || employment.role
      : employment.role || employment.company}
  {@const placeSecondary =
    employment.kind === 'project'
      ? employment.link
      : employment.role && employment.company
        ? employment.company
        : ''}
  <section class="flex flex-col gap-2">
    <header class="flex flex-wrap items-baseline gap-x-2">
      {#if editingEmploymentId === employment.id}
        <div class="flex w-full flex-col gap-2">
          <input class="rounded-md border border-border bg-background px-3 py-2 text-sm" bind:value={empName} placeholder={employment.kind === 'project' ? 'Project name' : 'Company'} />
          {#if employment.kind === 'job'}
            <div class="flex gap-2">
              <input class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm" bind:value={empRole} placeholder="Role" />
              <input class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm" bind:value={empLocation} placeholder="Location" />
            </div>
          {:else}
            <input class="rounded-md border border-border bg-background px-3 py-2 text-sm" bind:value={empLink} placeholder="https://…" />
          {/if}
          <div class="flex gap-2">
            <input class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm" bind:value={empStart} placeholder="Start" />
            <input class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm" bind:value={empEnd} placeholder="End" />
          </div>
          <textarea
            class="w-full resize-y rounded-md border border-border bg-background px-3 py-2 text-sm"
            rows="2"
            bind:value={empSummary}
            placeholder="Summary (optional)"
          ></textarea>
          <input class="rounded-md border border-border bg-background px-3 py-2 text-sm" bind:value={empStack} placeholder="Stack, comma-separated (optional)" />
          <div class="flex gap-2">
            <Button size="sm" disabled={busy} onclick={() => saveEmployment(employment)}>Save</Button>
            <Button size="sm" variant="ghost" onclick={() => (editingEmploymentId = null)}>Cancel</Button>
          </div>
        </div>
      {:else}
        <h3 class="text-sm font-semibold text-foreground">
          {placeLabel}
        </h3>
        {#if placeSecondary}
          <span class="text-sm text-muted-foreground">{placeSecondary}</span>
        {/if}
        {#if employment.location}
          <span class="text-sm text-muted-foreground">· {employment.location}</span>
        {/if}
        {#if employment.start || employment.end}
          <span class="text-xs text-muted-foreground">
            {employment.start}{employment.end ? ` – ${employment.end}` : ''}
          </span>
        {/if}
        <div class="ml-auto flex gap-1">
          <Button size="sm" variant="ghost" onclick={() => startEditEmployment(employment)}>
            <Pencil class="size-3.5" />
            Edit
          </Button>
          <Button
            size="icon"
            variant="ghost"
            onclick={() => requestRemoveEmployment(employment)}
            aria-label={`Remove ${placeLabel}`}
            class="text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
          >
            <Trash2 class="size-3.5" />
          </Button>
        </div>
      {/if}
    </header>

    {#if employment.summary}
      <p class="text-sm text-foreground">{employment.summary}</p>
    {/if}
    {#if employment.stack?.length}
      <div class="flex flex-wrap gap-1.5">
        {#each employment.stack as tech (tech)}
          <span class="rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">{tech}</span>
        {/each}
      </div>
    {/if}

    {#if employment.atoms.length === 0}
      <p class="text-sm text-muted-foreground">
        {#if employment.kind === 'project'}
          Nothing recorded for this project yet — the assistant can help you fill it in.
        {:else}
          Nothing recorded for this role yet — the assistant can help you fill it in.
        {/if}
      </p>
    {:else}
      <ul class="flex flex-col gap-1.5">
        {#each needsAttentionFirst(employment.atoms) as atom (atom.id)}
          {@render achievement(atom)}
        {/each}
      </ul>
    {/if}
  </section>
{/snippet}

<!-- The way into the interviewer. The label names what the owner gets, not the machine
     that produces it, and the example carries the rest: it shows the grain of an answer
     the interviewer is after — one result, ideally with a number — before the chat is
     even open. The caller wraps this to set its alignment.

     It opens the panel rather than navigating: the bank is what the conversation is
     about, and leaving it to reach the agent is what this replaced. -->
{#snippet interviewEntry(id: string)}
  <Button
    size="sm"
    variant="secondary"
    disabled={turnActive}
    onclick={() => launchInterview([])}
    aria-describedby={id}
  >
    Add an achievement
  </Button>
  <p {id} class="text-xs text-muted-foreground">
    Tell the assistant what you did — “I cut checkout latency by 40% in one quarter.”
  </p>
{/snippet}

{#snippet achievement(atom: ExperienceAtom)}
  <li
    class="group rounded-md border px-3 py-2 {unconfirmed(atom)
      ? 'border-warning/40 bg-warning/5'
      : selected.includes(atom.id)
        ? 'border-brand/50 bg-brand/5'
        : 'border-border'}"
  >
    {#if editing === atom.id}
      <div class="flex flex-col gap-2">
        <label class="flex flex-col gap-1">
          <span class="text-xs font-medium text-muted-foreground">Achievement</span>
          <textarea
            bind:value={draftClaim}
            rows="2"
            class="w-full resize-y rounded border border-border bg-background px-2 py-1 text-sm"
          ></textarea>
        </label>
        <label class="flex flex-col gap-1">
          <span class="text-xs font-medium text-muted-foreground">Context</span>
          <textarea
            bind:value={draftContext}
            rows="2"
            class="w-full resize-y rounded border border-border bg-background px-2 py-1 text-sm"
            placeholder="Where this happened — team, product, constraint…"
          ></textarea>
        </label>
        <label class="flex flex-col gap-1">
          <span class="text-xs font-medium text-muted-foreground">Metrics</span>
          <textarea
            bind:value={draftMetrics}
            rows="2"
            class="w-full resize-y rounded border border-border bg-background px-2 py-1 font-mono text-sm"
            placeholder="One per line or comma-separated — e.g. 40%"
          ></textarea>
        </label>
        <div class="flex items-center gap-2">
          <Button size="sm" onclick={() => saveEdit(atom)} disabled={busy || !draftClaim.trim()}>
            <Check class="size-4" />
            Save — this makes it yours
          </Button>
          <Button size="sm" variant="ghost" onclick={() => (editing = null)}>
            <X class="size-4" />
            Cancel
          </Button>
        </div>
      </div>
    {:else if promotingAtomId === atom.id}
      <div class="flex flex-col gap-2">
        <p class="text-sm text-foreground">{atom.claim}</p>
        <label class="flex flex-col gap-1">
          <span class="text-xs font-medium text-muted-foreground">Project name</span>
          <input
            class="rounded-md border border-border bg-background px-3 py-2 text-sm"
            bind:value={promoteName}
            placeholder="e.g. Sandrock, Git helpers"
          />
        </label>
        <label class="flex flex-col gap-1">
          <span class="text-xs font-medium text-muted-foreground">Link (optional)</span>
          <input
            class="rounded-md border border-border bg-background px-3 py-2 text-sm"
            bind:value={promoteLink}
            placeholder="https://"
          />
        </label>
        <div class="flex items-center gap-2">
          <Button size="sm" onclick={() => savePromoteToProject(atom)} disabled={busy || !promoteName.trim()}>
            <Check class="size-4" />
            Save as project
          </Button>
          <Button
            size="sm"
            variant="ghost"
            onclick={() => {
              promotingAtomId = null;
              promoteName = '';
              promoteLink = '';
            }}
          >
            <X class="size-4" />
            Cancel
          </Button>
        </div>
      </div>
    {:else}
      <div class="flex items-start gap-3">
        <input
          type="checkbox"
          class="mt-1 size-4 shrink-0 accent-brand"
          checked={selected.includes(atom.id)}
          onchange={() => toggleSelect(atom.id)}
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
                <span class="rounded bg-muted px-1.5 py-0.5 font-mono text-xs text-foreground">{metric}</span>
              {/each}
            </p>
          {/if}
          <p class="mt-1 text-xs text-muted-foreground">
            {provenanceLabel[atom.provenance]}
            {#if atom.skills?.length}
              · {atom.skills.join(', ')}
            {/if}
          </p>
          {#if atom.cluster_id || atom.needs_context || atom.needs_metrics}
            <p class="mt-1 flex flex-wrap gap-1.5">
              {#if atom.cluster_id}
                <span class="rounded border border-border px-1.5 py-0.5 text-xs text-muted-foreground">
                  Looks similar to another
                </span>
              {/if}
              {#if atom.needs_context}
                <span class="rounded border border-border px-1.5 py-0.5 text-xs text-muted-foreground">
                  Thin on context
                </span>
              {/if}
              {#if atom.needs_metrics}
                <span class="rounded border border-border px-1.5 py-0.5 text-xs text-muted-foreground">
                  No number yet
                </span>
              {/if}
            </p>
          {/if}
          {#if !atom.employment_id}
            <div class="mt-2">
              <Button
                size="sm"
                variant="secondary"
                onclick={() => startPromoteToProject(atom)}
                disabled={busy || turnActive}
              >
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
          {#if unconfirmed(atom)}
            <Button
              size="icon"
              variant="ghost"
              onclick={() => confirmAtom(atom)}
              disabled={busy}
              aria-label="Confirm achievement"
              class="text-muted-foreground hover:bg-brand-muted hover:text-brand-strong"
            >
              <Check class="size-4" />
            </Button>
          {/if}
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
            onclick={() => requestRemove(atom)}
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
