<script lang="ts">
  import { resolve } from '$app/paths';
  import { goto } from '$app/navigation';
  import { Button, cn, EntityLogo } from '$lib/ui';
  import { Trash2, X, ExternalLink, Mic, NotebookPen, Send, Target, SquarePen } from '@lucide/svelte';
  import { askConfirmTailor } from '$lib/confirmTailorDialog.svelte';
  import { groupedStages, humanizeStage, offersDebrief } from '$lib/stages';
  import { canFollowUp } from '$lib/followup';
  import { CLOSED_OUTCOMES, type ClosedOutcome } from '$lib/board';
  import { timeAgo, errorMessage } from '$lib/utils';
  import { tablist } from '$lib/actions/tablist';
  import { cardTagsFromCard } from '$lib/enrichment';
  import { companyLogoUrl } from '$lib/logo';
  import JobDescription from './JobDescription.svelte';
  import MatchAnalysisFull from './MatchAnalysisFull.svelte';
  import JobMatch from './JobMatch.svelte';
  import NoteEditor from './NoteEditor.svelte';
  import { api } from '$lib/api';
  import type { EmailBody } from '$lib/api';
  import { currentUser } from '$lib/auth.svelte';
  import { statusLabel, stageImplication } from '$lib/emailStatus';
  import { eventLabel, eventTone } from '$lib/events';
  import StatusChip from '$lib/components/StatusChip.svelte';
  import SkillChip from './SkillChip.svelte';
  import { avatarInitials, avatarColor } from '$lib/avatar';
  import type {
    Job,
    MyJob,
    ApplicationEmail,
    MailRecallResult,
    RecalledEmail,
    StageSuggestion,
    TimelineEvent
  } from '$lib/types';
  import { focusTrap } from '$lib/actions/focusTrap';
  import { lockScroll, unlockScroll } from '$lib/scrollLock';

  let {
    item,
    pendingOutcome,
    onsetstage,
    onsavenotes,
    onchooseoutcome,
    onremove,
    onclose,
    onrehearse,
    ondebrief,
    onfollowup,
    startingSession = false,
    sessionError = null,
    blocked = false,
  }: {
    item: MyJob;
    pendingOutcome: boolean;
    onsetstage: (stage: string) => void;
    onsavenotes: (notes: string) => void;
    onchooseoutcome: (o: ClosedOutcome) => void;
    onremove: () => void;
    onclose: () => void;
    // The actions the card used to carry. They live here because a card carries no
    // controls — one on its surface is what stopped it being dragged at all.
    onrehearse: (item: MyJob) => void;
    ondebrief: (item: MyJob) => void;
    onfollowup: (item: MyJob) => void;
    // Starting either conversation is one round trip before the navigation; the board
    // owns the flag so a second click in that window cannot mint a second one. Both
    // buttons share it because both end in the same navigation away from here.
    startingSession?: boolean;
    // ...and the board owns the failure too. It has to be rendered in here: this panel
    // covers the viewport, so a message left behind on the board would be invisible to
    // the person who pressed the button.
    sessionError?: string | null;
    // True while something is stacked above this panel — today, the follow-up dialog it
    // now opens. Both listen for Escape on the window, so without this one press would
    // close the dialog and the application underneath it in the same keystroke. The
    // parent is the only thing that knows what is stacked, so it says.
    blocked?: boolean;
  } = $props();

  // Tailoring navigates away, and /tailor/[slug] owns its own bootstrap — so the wait is
  // this component's to show.
  let tailoring = $state(false);

  async function startTailoring() {
    if (!item.job || tailoring) return;
    tailoring = true;
    const ok = await askConfirmTailor(item.job.public_slug, `${item.job.title} at ${item.job.company}`);
    if (!ok) {
      tailoring = false;
      return;
    }
    await goto(resolve('/tailor/[slug]', { slug: item.job.public_slug }));
  }

  // Rehearse, Analyze and Tailor all need the posting. Absent rather than disabled when
  // it is gone — the same treatment View job gets. Follow up does not need one: the chase
  // is addressed to the employer, which the application knows by itself.
  const hasPosting = $derived(!!item.job);
  const offersFollowUp = $derived(canFollowUp(item));
  // The debrief reviews an interview that has already happened, so it appears only once
  // the stage says one plausibly did. The backend takes it from any stage — this is
  // where to advertise it, not who may have it.
  const offersDebriefAction = $derived(hasPosting && offersDebrief(item.stage ?? ''));

  type Tab = 'application' | 'fit' | 'description' | 'emails';
  // The Emails tab shows linked mail — open to every signed-in user.
  const canSeeMail = $derived(!!currentUser());

  // Emails-tab state (declared before TABS, which shows the loaded count). The
  // application's linked mail lazy-loads (see the eager $effect below); each email
  // expands inline (accordion) to its full body, itself lazy-fetched. $state.raw —
  // these are API payloads we only ever reassign, never mutate.
  let emails = $state.raw<ApplicationEmail[] | null>(null);
  let emailsLoading = $state(false);
  let emailsError = $state<string | null>(null);
  // The server's read of what the mail implies but has not applied. Cleared the moment the
  // stage is set from here, so the offer disappears on the press rather than on the next
  // load — the server would stop sending it, but not until something asks it again.
  let stageSuggestion = $state.raw<StageSuggestion | null>(null);
  // The full posting. The listing serves a card — employer, role, and the facets a row draws —
  // because carrying every description was 84% of its payload for text no row renders. The
  // panel is the one place that wants the posting, and it already makes this request for the
  // linked mail, so the description arrives on a call that was happening anyway.
  let posting = $state.raw<Job | null>(null);
  let expandedId = $state<number | null>(null);
  let expandedBody = $state.raw<EmailBody | null>(null);
  let bodyLoading = $state(false);

  const TABS = $derived<{ id: Tab; label: string }[]>([
    { id: 'application', label: 'Application' },
    { id: 'fit', label: 'Job Match' },
    { id: 'description', label: 'Job description' },
    ...(canSeeMail ? [{ id: 'emails' as Tab, label: emails ? `Emails (${emails.length})` : 'Emails' }] : []),
  ]);
  // Local UI state. The parent re-keys this component per job (JobBoard's {#key}),
  // so a fresh mount always opens on Application.
  let tab = $state<Tab>('application');

  // The mailbox sweep: from this application, find the mail that belongs to it. What comes
  // back are SUGGESTIONS — nothing is linked — resolved by the same confirm/reject calls
  // the inbox uses. Null until the button is pressed, so an untouched tab shows no verdict.
  let recall = $state.raw<MailRecallResult | null>(null);
  let recallLoading = $state(false);
  let recallError = $state<string | null>(null);
  // Ids resolved since the sweep, so a confirmed or dismissed row leaves at the press
  // rather than on the next load.
  let recallResolved = $state.raw<string[]>([]);
  const recallPending = $derived(
    (recall?.suggested ?? []).filter((e) => !recallResolved.includes(e.provider_id ?? String(e.id)))
  );
  // Only applications can be swept: the window is anchored on the date it was recorded, and
  // a job that was merely saved has none.
  const canRecall = $derived(hasPosting && !!item.applied_at);

  async function loadEmails(force = false) {
    if ((emails !== null && !force) || emailsLoading) return;
    emailsLoading = true;
    emailsError = null;
    try {
      if (!item.job) return;
      const app = await api.getTrackedApplication(item.job.public_slug);
      emails = app.emails;
      stageSuggestion = app.stage_suggestion ?? null;
      posting = app.job;
      events = app.events ?? [];
    } catch (e) {
      emailsError = errorMessage(e, 'Failed to load emails.');
    } finally {
      emailsLoading = false;
    }
  }

  async function runRecall() {
    if (!item.job || recallLoading) return;
    recallLoading = true;
    recallError = null;
    recallResolved = [];
    try {
      recall = await api.recallApplicationMail(item.job.public_slug);
    } catch (e) {
      // Shown rather than swallowed. An empty result would read as "your mailbox holds
      // nothing", which is the wrong thing to say about a gateway being down.
      recall = null;
      recallError = errorMessage(e, 'Could not search your mail right now.');
    } finally {
      recallLoading = false;
    }
  }

  // A proposal arrives one of two ways and leaves the same way. From the mailbox search it
  // is a provider id and nothing of ours yet, so linking imports it first; from stored mail
  // it is a row carrying a suggestion, resolved by the calls the inbox already uses.
  async function resolveRecalled(e: RecalledEmail, accept: boolean) {
    const key = e.provider_id ?? String(e.id);
    recallResolved = [...recallResolved, key];
    try {
      if (e.provider_id) {
        if (accept && item.job) await api.linkRecalledMail(item.job.public_slug, e.provider_id);
      } else {
        await (accept ? api.confirmEmailLink(e.id) : api.rejectEmailLink(e.id));
      }
      if (accept) await loadEmails(true);
    } catch {
      // Put it back: a proposal the caller can press again beats a row that vanished
      // without becoming anything.
      recallResolved = recallResolved.filter((x) => x !== key);
    }
  }

  // Load the linked mail eagerly (moderator or beta only) so the tab shows its count
  // before it's opened. Re-keyed per job by the parent, so this runs once per application.
  $effect(() => {
    if (canSeeMail) void loadEmails();
  });

  async function toggleEmail(id: number) {
    if (expandedId === id) {
      expandedId = null;
      expandedBody = null;
      return;
    }
    expandedId = id;
    expandedBody = null;
    bodyLoading = true;
    try {
      expandedBody = await api.getEmail(id);
    } catch {
      /* leave the row collapsed-open with no body; the list still shows */
    } finally {
      bodyLoading = false;
    }
  }

  // Meta pills (work arrangement, region, employment type, seniority) — only the
  // stated ones, reusing the list-card logic.
  // The posting is gone once cmd/prune removes it. The employer and role are on the
  // application itself; everything else the drawer shows came from the posting and is
  // simply absent — the honest rendering, since we no longer have it to show.
  const company = $derived(item.job?.company || item.company_slug);
  const title = $derived(item.job?.title || item.role_title);
  let tags = $derived(item.job ? cardTagsFromCard(item.job) : []);
  let stageLabel = $derived(item.stage ? humanizeStage(item.stage) : null);

  // The application's history, newest first, from the ledger. It replaces a strip that read
  // as a timeline and was not one: viewed/saved/applied ordered by depth, so the newest fact
  // sat on the left and the oldest on the right. `viewed` and `saved` are gone from it on
  // purpose — they are marks on a posting, and viewed_at is refreshed on every view, so at
  // the foot of a history it would state a first view while holding the latest date.
  let events = $state.raw<TimelineEvent[]>([]);

  // Lock background scroll while the fullscreen panel is open, released on unmount
  // (close / job switch). A DOM side-effect — the legitimate use of $effect.
  //
  // Through the shared reference-counted lock rather than by writing body.overflow here: a
  // direct write is exactly what desynchronizes a refcount that only acts on the 0↔1
  // transition. The drawer happens to cover the header today (fixed inset-0 z-50), so nothing
  // else can be holding the lock — but that is a layout fact, and the next layout change should
  // not be able to leave the page unscrollable.
  $effect(() => {
    lockScroll();
    return () => unlockScroll();
  });

  // Close the panel, first blurring whatever is focused so a pending notes edit is
  // flushed (the editor saves on blur) while the parent's openItem is still set —
  // onclose clears it, and the deferred save reads openItem, so it must run first.
  function close() {
    (document.activeElement as HTMLElement | null)?.blur();
    onclose();
  }

  const tabClass = (active: boolean) =>
    cn(
      'shrink-0 rounded-full px-4 py-1.5 text-sm transition-colors',
      active
        ? 'bg-card font-medium text-foreground shadow-sm'
        : 'text-muted-foreground hover:text-foreground',
    );
  const sectionLabel = 'text-sm font-medium text-muted-foreground';
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && !blocked && close()} />

<!-- Fullscreen job panel (like the swipe deck): a centered column with a fixed
     header + pill tabs, a scrolling tab body, and a pinned View-job footer. -->
<div
  class="fixed inset-0 z-50 flex flex-col bg-background text-foreground"
  role="dialog"
  aria-modal="true"
  aria-label="Job details"
  {@attach focusTrap()}
>
  <!-- Header: logo · title · company · close, then meta pills and tabs -->
  <div class="shrink-0 border-b border-border">
    <div class="mx-auto flex w-full max-w-2xl flex-col gap-4 px-5 pb-3 pt-5 sm:px-6">
      <div class="flex items-start gap-4">
        <div class="flex size-12 shrink-0 items-center justify-center overflow-hidden rounded-2xl">
          <EntityLogo
            name={company || 'Unknown company'}
            src={companyLogoUrl(company) ?? undefined}
            shape="square"
            size="md"
          />
        </div>
        <div class="min-w-0 flex-1">
          <h2 class="text-xl font-bold leading-tight tracking-tight">{title}</h2>
          <p class="text-sm text-muted-foreground">{company || 'Unknown company'}</p>
        </div>
        <div class="flex shrink-0 items-center gap-2">
          {#if item.job}
          <Button
            variant="outline"
            size="sm"
            href={resolve('/jobs/[slug]', { slug: item.job.public_slug })}
            target="_blank"
            rel="noopener noreferrer"
            class="gap-1.5 whitespace-nowrap"
          >
            View job
            <ExternalLink class="size-3.5" />
          </Button>
          {/if}
          <button
            type="button"
            onclick={close}
            class="-mr-1 rounded-full p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
            aria-label="Close"
          >
            <X class="size-5" />
          </button>
        </div>
      </div>

      {#if tags.length || stageLabel}
        <div class="flex flex-wrap items-center gap-2">
          {#each tags as tag (tag)}
            <span class="rounded-full bg-muted px-2.5 py-0.5 text-xs font-medium text-muted-foreground">{tag}</span>
          {/each}
          {#if stageLabel}
            <span class="rounded-full bg-brand-muted px-2.5 py-0.5 text-xs font-medium text-brand-strong">{stageLabel}</span>
          {/if}
        </div>
      {/if}

      <!-- What the candidate can do about this application, on every tab. The card used
           to carry Rehearse and Follow up in a strip beside its badges, which is both
           less room than they need and the reason the card could not be dragged. -->
      {#if hasPosting || offersFollowUp}
        <div class="flex flex-wrap items-center gap-2">
          {#if hasPosting}
            <Button variant="outline" size="sm" onclick={() => onrehearse(item)} disabled={startingSession} class="gap-1.5">
              <Mic class="size-3.5" />
              {startingSession ? 'Starting…' : 'Rehearse'}
            </Button>
          {/if}
          {#if offersDebriefAction}
            <!-- Sits next to Rehearse: the pair reads as before and after the interview. -->
            <Button variant="outline" size="sm" onclick={() => ondebrief(item)} disabled={startingSession} class="gap-1.5">
              <NotebookPen class="size-3.5" />
              {startingSession ? 'Starting…' : 'Debrief'}
            </Button>
          {/if}
          {#if offersFollowUp}
            <Button
              variant="outline"
              size="sm"
              onclick={() => onfollowup(item)}
              class="gap-1.5 border-transparent bg-warning-muted text-warning-strong hover:bg-warning-muted hover:opacity-80"
            >
              <Send class="size-3.5" />
              {item.followed_up_at ? 'Chase again' : 'Follow up'}
            </Button>
          {/if}
          {#if hasPosting}
            <!-- The analysis is already in this panel; sending the user to /tailor would
                 close the application to show them something it contains. -->
            <Button variant="outline" size="sm" onclick={() => (tab = 'fit')} class="gap-1.5">
              <Target class="size-3.5" />
              Analyze
            </Button>
            <Button variant="outline" size="sm" onclick={startTailoring} disabled={tailoring} class="gap-1.5">
              <SquarePen class="size-3.5" />
              {tailoring ? 'Preparing…' : 'Tailor CV'}
            </Button>
          {/if}
        </div>
      {/if}
      {#if sessionError}
        <p class="text-sm text-warning-strong" role="alert">{sessionError}</p>
      {/if}

      <div class="no-scrollbar overflow-x-auto">
        <div
          role="tablist"
          aria-label="Job details view"
          use:tablist={tab}
          class="flex w-max items-center gap-1 rounded-full bg-muted p-1"
        >
          {#each TABS as t (t.id)}
            <button
              type="button"
              role="tab"
              id="jobdrawer-tab-{t.id}"
              aria-selected={tab === t.id}
              aria-controls="jobdrawer-tabpanel"
              class={tabClass(tab === t.id)}
              onclick={() => (tab = t.id)}
            >
              {t.label}
            </button>
          {/each}
        </div>
      </div>
    </div>
  </div>

  <!-- Scrolling tab body -->
  <div class="min-h-0 flex-1 overflow-y-auto">
    <div
      role="tabpanel"
      id="jobdrawer-tabpanel"
      aria-labelledby="jobdrawer-tab-{tab}"
      tabindex="0"
      class="mx-auto w-full max-w-2xl px-5 py-5 sm:px-6"
    >
      {#if tab === 'application'}
        <div class="flex flex-col gap-4">
          {#if pendingOutcome}
            <div class="flex flex-col gap-2 rounded-lg border border-border p-3">
              <p class="text-sm font-medium">How did it close?</p>
              <div class="flex flex-wrap gap-2">
                {#each CLOSED_OUTCOMES as o (o)}
                  <Button variant="outline" onclick={() => onchooseoutcome(o)}>{humanizeStage(o)}</Button>
                {/each}
              </div>
            </div>
          {/if}

          <!-- What happened, newest first. Absent entirely when the ledger holds nothing:
               an application saved but never applied to has no history, and an empty frame
               would say otherwise. -->
          {#if events.length}
            <div class="flex flex-col gap-1 text-sm">
              <span class="font-medium">History</span>
              <ol class="flex flex-col gap-1.5">
                {#each events as e (e.id)}
                  <li class="flex items-baseline gap-2">
                    <span class="shrink-0 text-xs {eventTone(e.kind)}" aria-hidden="true">●</span>
                    <span class="w-24 shrink-0 text-xs text-muted-foreground">{timeAgo(e.occurred_at)}</span>
                    <span class="min-w-0 text-sm">{eventLabel(e)}</span>
                  </li>
                {/each}
              </ol>
            </div>
          {/if}

          <label class="flex flex-col gap-1 text-sm">
            <span class="font-medium">Stage</span>
            <select
              value={item.stage ?? ''}
              onchange={(e) => onsetstage(e.currentTarget.value)}
              class="rounded-md border border-input bg-transparent px-2 py-1.5 text-sm"
            >
              <option value="">No stage</option>
              <!-- Grouped so `Closed` reads as a heading over its three outcomes rather than
                   as a fifth state competing with them — the same four groups the board's
                   columns use, from the same generated table. -->
              {#each groupedStages() as g (g.id)}
                <optgroup label={g.label}>
                  {#each g.options as s (s.value)}
                    <option value={s.value}>{s.label}</option>
                  {/each}
                </optgroup>
              {/each}
            </select>
          </label>

          <div class="flex flex-col gap-1 text-sm">
            <span class="font-medium">Notes</span>
            <NoteEditor value={item.notes ?? ''} onsave={onsavenotes} />
          </div>
        </div>
      {:else if tab === 'fit'}
        <div class="flex flex-col gap-6">
          {#if posting}
            <JobMatch job={posting} />
            <MatchAnalysisFull job={posting} />
          {:else if item.job}
            <p class="text-sm text-muted-foreground">Loading…</p>
          {/if}
        </div>
      {:else if tab === 'emails'}
        <div class="flex flex-col gap-2">
          <!-- The mail says one thing, the stage says another, and nothing moved it. That is
               the rule working — mail never settles an application — so the resolution is
               offered rather than applied, and it goes through the ordinary stage change so
               the ledger records the candidate as its source. -->
          {#if stageSuggestion}
            <div class="flex flex-wrap items-center gap-2 rounded-md border border-warning/50 bg-warning-muted/40 px-3 py-2">
              <span class="min-w-0 flex-1 text-sm">
                This looks like <span class="font-medium">{statusLabel(stageSuggestion.signal).toLowerCase()}</span>,
                but the stage is
                <span class="font-medium">{item.stage ? humanizeStage(item.stage) : 'unset'}</span>.
              </span>
              <Button
                size="sm"
                variant="outline"
                class="shrink-0"
                onclick={() => {
                  const stage = stageSuggestion?.stage;
                  stageSuggestion = null;
                  if (stage) onsetstage(stage);
                }}
              >
                Move to {humanizeStage(stageSuggestion.stage)}
              </Button>
              <button
                type="button"
                class="shrink-0 text-xs text-muted-foreground underline-offset-2 hover:underline"
                onclick={() => (stageSuggestion = null)}
              >
                Dismiss
              </button>
            </div>
          {/if}
          <!-- The pull direction. Everything else in the mail stack starts from a message
               and asks which application it belongs to; this asks the opposite, which is
               the only way an application that plainly ought to have mail can say so. -->
          {#if canRecall}
            <div class="flex flex-wrap items-center gap-2">
              <Button size="sm" variant="outline" disabled={recallLoading} onclick={runRecall}>
                {recallLoading ? 'Searching your mail…' : 'Find this application’s mail'}
              </Button>
              {#if recall && recallPending.length === 0}
                <span class="text-xs text-muted-foreground">
                  {recall.scanned === 0
                    ? 'No unattached mail around this application to look at.'
                    : `Nothing matched among ${recall.scanned} message${recall.scanned === 1 ? '' : 's'}.`}
                </span>
              {/if}
            </div>
          {/if}
          {#if recallError}
            <p class="text-sm text-destructive">{recallError}</p>
          {/if}
          {#if recallPending.length > 0}
            <div class="flex flex-col gap-2 rounded-xl border border-dashed border-border p-3">
              <p class="text-sm">
                <span class="font-medium">{recallPending.length}</span>
                of {recall?.scanned} message{recall?.scanned === 1 ? '' : 's'} may belong here.
                Nothing is attached until you say so.
              </p>
              <!-- Said where the boundary is crossed, not buried in a settings page: the
                   sweep looks through the mailbox and keeps nothing until Link is pressed. -->
              <p class="text-xs text-muted-foreground">
                This searched your mailbox for {company}. Nothing is saved unless you link it.
              </p>
              {#each recallPending as e (e.provider_id ?? e.id)}
                <div class="flex flex-wrap items-center gap-2 rounded-md border border-border px-3 py-2">
                  <div class="min-w-0 flex-1">
                    <div class="flex items-baseline gap-2">
                      <span class="min-w-0 flex-1 truncate text-sm font-medium">{e.from_name || e.from_addr}</span>
                      <span class="shrink-0 text-xs text-muted-foreground">{timeAgo(e.received_at)}</span>
                    </div>
                    <div class="truncate text-sm text-muted-foreground">{e.subject || '(no subject)'}</div>
                    <!-- Marked on the row it belongs to, not only counted below it. The
                         count alone made the reader hunt for which message it meant. -->
                    {#if e.invitation}
                      <span class="mt-1 inline-flex text-xs text-muted-foreground">
                        Carries a calendar invitation
                      </span>
                    {/if}
                  </div>
                  <Button size="sm" class="shrink-0" onclick={() => resolveRecalled(e, true)}>Link</Button>
                  <button
                    type="button"
                    class="shrink-0 text-xs text-muted-foreground underline-offset-2 hover:underline"
                    onclick={() => resolveRecalled(e, false)}
                  >
                    Not this one
                  </button>
                </div>
              {/each}
              <!-- The calendar follows from the mail and needs no calendar code: cal-sync
                   re-reads its whole window every run, so an invitation linked today
                   produces its meeting on the next one. -->
              {#if recall && recall.invitations > 0}
                <p class="text-xs text-muted-foreground">
                  Linking an invitation brings its meeting onto your calendar view after the next
                  sync.
                </p>
              {/if}
            </div>
          {/if}
          {#if emailsLoading}
            <p class="text-sm text-muted-foreground">Loading emails…</p>
          {:else if emailsError}
            <p class="text-sm text-destructive">{emailsError}</p>
          {:else if !emails || emails.length === 0}
            <p class="text-sm text-muted-foreground">No emails linked to this application yet.</p>
          {:else}
            {#each emails as e (e.id)}
              <div class="overflow-hidden rounded-xl border border-border">
                <button
                  type="button"
                  onclick={() => toggleEmail(e.id)}
                  aria-expanded={expandedId === e.id}
                  class="flex w-full items-start gap-3 p-3 text-left transition-colors hover:bg-accent"
                >
                  <div
                    class="mt-0.5 flex size-9 shrink-0 select-none items-center justify-center rounded-full text-xs font-semibold text-white"
                    style="background-color: {avatarColor(e.from_addr || e.from_name)}"
                  >
                    {avatarInitials(e.from_name, e.from_addr)}
                  </div>
                  <div class="min-w-0 flex-1">
                    <div class="flex items-baseline gap-2">
                      <span class="min-w-0 flex-1 truncate text-sm font-medium">{e.from_name || e.from_addr}</span>
                      <span class="shrink-0 text-[11px] text-muted-foreground">{timeAgo(e.received_at)}</span>
                    </div>
                    <div class="mt-0.5 truncate text-sm text-muted-foreground">{e.subject || '(no subject)'}</div>
                    {#if statusLabel(e.status_signal)}
                      <!-- The chip, and what its signal means for the stage. The chip alone left
                           three different situations looking identical: the signal moved the
                           stage, it named one only the candidate may apply, or it was never
                           about progress.
                           Both run at the chip's own size (text-xs) rather than at a 10px the
                           wrapper used to impose: StatusChip renders a Badge, which carries its
                           size, so pinning the wrapper smaller would leave the explanation off
                           the chip's baseline — and this thread has the room the dense inbox
                           list does not. -->
                      <span class="mt-1 inline-flex flex-wrap items-baseline gap-1.5 text-xs leading-4">
                        <StatusChip signal={e.status_signal} />
                        {#if stageImplication(e.status_signal)}
                          <span class="text-muted-foreground">{stageImplication(e.status_signal)}</span>
                        {/if}
                      </span>
                    {/if}
                  </div>
                </button>
                {#if expandedId === e.id}
                  <div class="border-t border-border p-3">
                    {#if bodyLoading}
                      <p class="text-sm text-muted-foreground">Loading…</p>
                    {:else if expandedBody?.body_html}
                      <!-- Untrusted sender HTML isolated in a sandboxed iframe (no scripts/forms/navigation). -->
                      <iframe
                        title="Message body"
                        sandbox=""
                        srcdoc={expandedBody.body_html}
                        class="h-96 w-full bg-white"
                      ></iframe>
                    {:else if expandedBody?.body_text}
                      <pre class="max-h-96 overflow-y-auto whitespace-pre-wrap font-sans text-sm leading-relaxed">{expandedBody.body_text}</pre>
                    {:else}
                      <p class="text-sm text-muted-foreground">No content.</p>
                    {/if}
                  </div>
                {/if}
              </div>
            {/each}
          {/if}
        </div>
      {:else}
        <div class="flex flex-col gap-5">
          {#if !item.job}
            <!-- The posting was removed from the catalogue. What it said is genuinely
                 gone; saying so is better than an empty tab that reads as a bug. -->
            <p class="text-sm text-muted-foreground">
              This posting is no longer listed. Your application, its stage and your notes are kept.
            </p>
          {:else if posting?.description}
            <JobDescription html={posting.description} />
          {:else if !posting}
            <p class="text-sm text-muted-foreground">Loading…</p>
          {:else}
            <p class="text-sm text-muted-foreground">No description available.</p>
          {/if}

          {#if posting?.skills?.length}
            <div class="flex flex-col gap-2 border-t border-border pt-5">
              <p class={sectionLabel}>Skills</p>
              <div class="flex flex-wrap gap-1.5">
                <!-- Unlinked: the drawer is a reading surface over a posting, and a
                     filter link would navigate out of it. -->
                {#each posting.skills as skill (skill)}
                  <SkillChip slug={skill} linked={false} />
                {/each}
              </div>
            </div>
          {/if}
        </div>
      {/if}
    </div>
  </div>

  <!-- Pinned footer: remove-from-board (destructive card action) -->
  <div class="shrink-0 border-t border-border">
    <div class="mx-auto flex w-full max-w-2xl justify-end px-5 py-3 sm:px-6">
      <Button
        variant="ghost"
        size="sm"
        onclick={onremove}
        class="gap-1.5 text-red-600 hover:bg-red-50 hover:text-red-700 dark:text-red-400 dark:hover:bg-red-950/40 dark:hover:text-red-300"
      >
        <Trash2 class="size-4" />
        Remove from board
      </Button>
    </div>
  </div>
</div>
