<script lang="ts">
  import { untrack } from 'svelte';
  import { resolve } from '$app/paths';
  import { goto } from '$app/navigation';
  import { Button, ConfirmDialog, cn, EntityLogo } from '#lib/ui/index.js';
  import { Trash2, X, ExternalLink, Send, SquarePen } from '@lucide/svelte';
  import { askConfirmTailor, confirmTailorDialog } from '#lib/confirmTailorDialog.svelte.js';
  import { tailorHref } from '#lib/tailorHref.js';
  import { groupedStages, humanizeStage } from '#lib/stages.js';
  import { canFollowUp } from '#lib/followup.js';
  import { CLOSED_OUTCOMES, type ClosedOutcome } from '#lib/board.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { timeAgo, errorMessage } from '#lib/utils.js';
  import { tablist } from '#lib/actions/tablist.js';
  import { ScrollEdges } from '#lib/scrollEdges.svelte.js';
  import { cardTagsFromCard } from '#lib/enrichment.js';
  import { companyLogoUrl } from '#lib/logo.js';
  import JobDescription from './JobDescription.svelte';
  import MatchAnalysisFull from './MatchAnalysisFull.svelte';
  import JobMatch from './JobMatch.svelte';
  import NoteEditor from './NoteEditor.svelte';
  import { api } from '#lib/api.js';
  import JobDrawerMailRecall from './JobDrawerMailRecall.svelte';
  import JobDrawerEmailList from './JobDrawerEmailList.svelte';
  import JobDrawerAutoApply from './JobDrawerAutoApply.svelte';
  import { currentUser } from '#lib/auth.svelte.js';
  import { statusLabel } from '#lib/emailStatus.js';
  import { eventLabel, eventTone } from '#lib/events.js';
  import SkillChip from './SkillChip.svelte';
  import type {
    Job,
    MyJob,
    ApplicationEmail,
    AutoApplyReviewInfo,
    StageSuggestion,
    TimelineEvent,
  } from '#lib/types.js';
  import { focusTrap } from '#lib/actions/focusTrap.js';
  import { lockScroll, unlockScroll } from '#lib/ui/index.js';
  import { messages } from './JobDrawer.messages';
  import JobDrawerLoadState from './JobDrawerLoadState.svelte';
  import AssistantChat from '#lib/assistant/AssistantChat.svelte';
  import AssistantPanel from '#lib/components/AssistantPanel.svelte';
  import { openApplyChat } from '#lib/assistant/api.js';
  import { format, t } from '#lib/i18n/t.js';

  let {
    item,
    initialTab = 'application',
    pendingOutcome,
    onsetstage,
    onsavenotes,
    onchooseoutcome,
    onautoapplyreview,
    onremove,
    onclose,
    onfollowup,
    blocked = false,
  }: {
    item: MyJob;
    initialTab?: 'application' | 'auto_apply';
    pendingOutcome: boolean;
    onsetstage: (stage: string) => void;
    // Rejects when the write fails; the notes editor reports it and keeps the text.
    onsavenotes: (notes: string) => Promise<void>;
    onchooseoutcome: (o: ClosedOutcome) => void;
    // Called after a successful approve/decline so the board's own item (which this
    // component only ever borrows through the `item` prop) gets its badge updated —
    // mirroring onsetstage/onsavenotes: the panel makes the call because it alone holds
    // the queue id (loaded via getTrackedApplication), but the mutation itself belongs to
    // whichever component owns `item`'s reactivity, the same division of labor every other
    // mutation here already follows.
    onautoapplyreview: (status: 'approved' | 'declined') => void;
    // Runs behind a confirmation, and may throw: its message is shown in the dialog.
    onremove: () => Promise<void>;
    onclose: () => void;
    // The actions the card used to carry. They live here because a card carries no
    // controls — one on its surface is what stopped it being dragged at all.
    onfollowup: (item: MyJob) => void;
    // True while something is stacked above this panel — today, the follow-up dialog it
    // now opens. Both listen for Escape on the window, so without this one press would
    // close the dialog and the application underneath it in the same keystroke. The
    // parent is the only thing that knows what is stacked, so it says.
    blocked?: boolean;
  } = $props();

  const s = $derived(t(messages, locale()));
  let buddyVisibilityError = $state<string | null>(null);

  // Removing untracks the application — its stage, notes and history go with it — so it is
  // asked first, the way deleting a list is.
  let confirmRemoveOpen = $state(false);
  const jobLabel = $derived(item.job?.title || item.role_title || s.removeUntitled);

  // Tailoring navigates away, and /tailor/[slug] owns its own bootstrap — so the wait is
  // this component's to show.
  let tailoring = $state(false);

  async function startTailoring() {
    if (!item.job || tailoring) return;
    tailoring = true;
    const choice = await askConfirmTailor(item.job.public_slug, `${item.job.title} at ${item.job.company}`);
    if (!choice) {
      tailoring = false;
      return;
    }
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- tailorHref() wraps resolve('/tailor/[slug]'); the rule can't see through the appended ?base= query
    await goto(tailorHref(item.job.public_slug, choice.baseCvId));
  }

  // Tailoring needs the posting. Absent rather than disabled when
  // it is gone — the same treatment View job gets. Follow up does not need one: the chase
  // is addressed to the employer, which the application knows by itself.
  const hasPosting = $derived(!!item.job);
  const offersFollowUp = $derived(canFollowUp(item));
  type Tab = 'application' | 'history' | 'fit' | 'description' | 'emails' | 'auto_apply';
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
  // The caller's own live auto-apply attempt for this job, loaded alongside everything
  // else loadEmails() fetches (openspec/changes/auto-apply-review-tracking) — null for a
  // job with none.
  let autoApply = $state.raw<AutoApplyReviewInfo | null>(null);

  let applyChatId = $state<string | null>(null);
  let applyChatOpen = $state(false);
  let applyChatError = $state<string | null>(null);

  async function startApplyChat() {
    if (!item.job) return;
    applyChatError = null;
    try {
      applyChatId ??= (await openApplyChat(item.job.public_slug)).id;
      applyChatOpen = true;
    } catch (e) {
      applyChatError = errorMessage(e, s.applyChat.openFailed);
    }
  }

  // The application's history, newest first, from the ledger — also loaded by
  // loadEmails() below. It replaces a strip that read as a timeline and was not one:
  // viewed/saved/applied ordered by depth, so the newest fact sat on the left and the
  // oldest on the right. `viewed` and `saved` are gone from it on purpose — they are marks
  // on a posting, and viewed_at is refreshed on every view, so at the foot of a history it
  // would state a first view while holding the latest date.
  //
  // Declared here (before TABS, which reads autoApplied) rather than nearer its own
  // rendering further down — the same ordering `emails`'s own comment already explains.
  let events = $state.raw<TimelineEvent[]>([]);
  let buddyMember = $state(false);
  let hiddenInBuddyFeed = $state(false);
  let buddyVisibilityBusy = $state(false);
  const buddyApplicationId = $derived(events.find((event) => event.application_id)?.application_id ?? null);
  // One request answers both questions: the hidden list is a member-only route (403 for
  // anybody else), so its answer is the membership too. Never GET /me/buddies from here —
  // that one refreshes the member's ranking keys and belongs to entering the section.
  // Nothing is asked until the drawer holds an application, so a posting never applied to
  // costs no request.
  $effect(() => {
    const id = buddyApplicationId;
    if (id !== null) void loadBuddyVisibility(id);
  });
  async function loadBuddyVisibility(id: number) {
    try {
      const ids = await api.listBuddyHiddenApplications();
      buddyMember = true;
      hiddenInBuddyFeed = ids.includes(id);
    } catch {
      buddyMember = false;
    }
  }
  async function toggleBuddyVisibility() {
    if (buddyApplicationId === null || buddyVisibilityBusy) return;
    buddyVisibilityBusy = true;
    const previous = hiddenInBuddyFeed;
    hiddenInBuddyFeed = !previous;
    try {
      await api.setBuddyApplicationHidden(String(buddyApplicationId), hiddenInBuddyFeed);
    } catch (err) {
      hiddenInBuddyFeed = previous;
      buddyVisibilityError = err instanceof Error ? err.message : s.errors.buddyVisibility;
    } finally {
      buddyVisibilityBusy = false;
    }
  }

  // Once a successful submission retires the live attempt, `autoApply` goes back to null
  // and the job's stage looks like any other "applied" job. The ledger still knows better:
  // cmd/auto-apply's Store.Submit records this exact fact (appevent.SourceAutoApply) when
  // it calls MarkJobApplied. See openspec/changes/auto-apply-progress-tab/design.md.
  const autoApplied = $derived(events.some((e) => e.kind === 'applied' && e.source === 'auto_apply'));

  // The full posting. The listing serves a card — employer, role, and the facets a row draws —
  // because carrying every description was 84% of its payload for text no row renders. The
  // panel is the one place that wants the posting, and it already makes this request for the
  // linked mail, so the description arrives on a call that was happening anyway.
  let posting = $state.raw<Job | null>(null);

  const TABS = $derived<{ id: Tab; label: string }[]>([
    { id: 'application', label: s.tabs.application },
    ...(autoApply || autoApplied || (hasPosting && initialTab === 'auto_apply' && emails === null)
      ? [{ id: 'auto_apply' as Tab, label: s.tabs.progress }]
      : []),
    { id: 'fit', label: s.tabs.jobMatch },
    { id: 'description', label: s.tabs.jobDescription },
    { id: 'history', label: s.history },
    ...(canSeeMail
      ? [
          {
            id: 'emails' as Tab,
            label: emails ? format(s.tabs.emailsWithCount, { count: String(emails.length) }) : s.tabs.emails,
          },
        ]
      : []),
  ]);
  // The parent re-keys the drawer per job. A deep link chooses the opening tab;
  // later tab navigation belongs to this drawer.
  // svelte-ignore state_referenced_locally
  let tab = $state<Tab>(initialTab === 'auto_apply' && !item.job ? 'application' : initialTab);

  // Only applications can be swept: the window is anchored on the date it was recorded, and
  // a job that was merely saved has none.
  const canRecall = $derived(hasPosting && !!item.applied_at);

  async function loadEmails(force = false) {
    if (!item.job || (emails !== null && !force) || emailsLoading) return;
    emailsLoading = true;
    emailsError = null;
    try {
      const app = await api.getTrackedApplication(item.job.public_slug);
      emails = app.emails;
      stageSuggestion = app.stage_suggestion ?? null;
      autoApply = app.auto_apply ?? null;
      posting = app.job;
      events = app.events ?? [];
      if (tab === 'auto_apply' && !autoApply && !autoApplied) tab = 'application';
    } catch (e) {
      emailsError = errorMessage(e, s.errors.loadEmails);
    } finally {
      emailsLoading = false;
    }
  }

  // Load the linked mail eagerly (moderator or beta only) so the tab shows its count
  // before it's opened. Re-keyed per job by the parent, so this runs once per application.
  $effect(() => {
    if (canSeeMail) untrack(() => void loadEmails());
  });

  // Meta pills (work arrangement, region, employment type, seniority) — only the
  // stated ones, reusing the list-card logic.
  // The posting is gone once it is deleted. The employer and role are on the
  // application itself; everything else the drawer shows came from the posting and is
  // simply absent — the honest rendering, since we no longer have it to show.
  const company = $derived(item.job?.company || item.company_slug);
  const title = $derived(item.job?.title || item.role_title);
  let tags = $derived(item.job ? cardTagsFromCard(item.job) : []);
  let stageLabel = $derived(item.stage ? humanizeStage(item.stage, locale()) : null);
  // Tailoring is what the drawer leads with while the application is still being prepared —
  // the same call to action the job page makes. Once it has been sent, a tailored CV is no
  // longer the next step, and every action here weighs the same.
  const tailorIsPrimary = $derived(!item.applied_at);

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
  // flushed (the editor saves on blur) while the panel and its "Saving…" are still
  // on screen; the editor's teardown save would catch it too, but out of sight.
  function close() {
    (document.activeElement as HTMLElement | null)?.blur();
    onclose();
  }

  // The tab strip outgrows a phone ("Описание вакансии" was cut to "Описани" with nothing
  // saying the row scrolls), so it fades whichever edge has tabs beyond it and keeps the
  // selected tab in view.
  let tabStrip = $state<HTMLElement | null>(null);
  const tabEdges = new ScrollEdges();
  $effect(() => {
    void tab;
    const active = tabStrip?.querySelector<HTMLElement>('[aria-selected="true"]');
    if (active) tabEdges.reveal(active);
  });

  // Actions keep their natural width, including when only Tailor CV is offered.
  const actionClass = 'h-9 min-w-0 gap-1.5 px-3 sm:h-8';

  const tabClass = (active: boolean) =>
    cn(
      'min-h-11 shrink-0 border-b-2 px-1 py-2 text-xs font-medium transition-colors sm:px-2 sm:text-sm',
      active ? 'border-foreground text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground',
    );
  const sectionLabel = 'text-sm font-medium text-muted-foreground';
</script>

<!-- The confirmation is stacked above the panel too: its Escape closes it, not both. -->
<!-- Not while the application chat is open: Escape there belongs to the chat (the covering
     panel closes itself on it), and closing the whole drawer under a half-typed answer would
     throw the conversation away. -->
<svelte:window
  onkeydown={(e) =>
    e.key === 'Escape' && !blocked && !confirmRemoveOpen && !applyChatOpen && !confirmTailorDialog.open && close()}
/>

<!-- A focused desktop dialog; the same application uses the full screen on phones. -->
<div class="fixed inset-0 z-50 bg-foreground/30" aria-hidden="true"></div>
<div
  class="fixed inset-0 z-50 flex animate-modal-fade flex-col overflow-hidden bg-background text-foreground outline-none sm:inset-6 sm:mx-auto sm:my-auto sm:max-h-[52rem] sm:max-w-3xl sm:rounded-2xl sm:border sm:border-border sm:shadow-xl"
  role="dialog"
  aria-modal="true"
  aria-label={s.dialogAria}
  {@attach focusTrap()}
>
  <!-- Employer, role, and posting details lead; status and application actions follow. -->
  <div class="shrink-0 border-b border-border">
    <div class="mx-auto flex w-full max-w-2xl flex-col gap-3 px-5 pt-4 sm:gap-4 sm:px-6 sm:pt-5">
      <div class="flex items-center gap-3">
        <EntityLogo
          name={company || s.unknownCompany}
          src={companyLogoUrl(company) ?? undefined}
          shape="square"
          size="sm"
          class="shrink-0"
        />
        <p class="min-w-0 flex-1 break-words text-sm font-medium text-muted-foreground">
          {company || s.unknownCompany}
        </p>
        {#if item.job}
          <a
            href={resolve('/jobs/[slug]', { slug: item.job.public_slug })}
            target="_blank"
            rel="noopener noreferrer"
            class="inline-flex min-h-11 shrink-0 items-center gap-1.5 rounded px-1 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring sm:min-h-8 sm:text-sm"
          >
            <ExternalLink class="size-3.5" aria-hidden="true" />
            {s.viewJob}
          </a>
        {/if}
        <button
          type="button"
          onclick={close}
          class="-mr-1.5 flex size-11 shrink-0 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-foreground sm:size-8"
          aria-label={s.close}
        >
          <X class="size-5" />
        </button>
      </div>

      <h2 class="text-balance text-xl font-semibold leading-tight tracking-tight">{title}</h2>

      {#if tags.length}
        <p class="flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
          {#each tags as tag (tag)}
            <span>{tag}</span>
          {/each}
        </p>
      {/if}

      {#if stageLabel || item.job || offersFollowUp}
        <div class="flex flex-wrap items-center gap-x-3 gap-y-2">
          {#if stageLabel}
            <span class="inline-flex items-center gap-1.5 text-xs font-medium text-foreground">
              <span class="size-1.5 shrink-0 rounded-full bg-brand" aria-hidden="true"></span>
              {stageLabel}
            </span>
          {/if}
          <div class="ml-auto flex flex-wrap items-center justify-end gap-2">
            {#if item.job}
              <Button
                variant={tailorIsPrimary ? 'primary' : 'outline'}
                size="sm"
                onclick={startTailoring}
                disabled={tailoring}
                class={actionClass}
              >
                <SquarePen class="size-3.5 shrink-0" />
                <span class="truncate">{tailoring ? s.preparing : s.tailorCv}</span>
              </Button>
            {/if}
            {#if offersFollowUp}
              <Button
                variant="outline"
                size="sm"
                onclick={() => onfollowup(item)}
                class={cn(
                  actionClass,
                  'border-transparent bg-warning-muted text-warning-strong hover:bg-warning-muted hover:opacity-80',
                )}
              >
                <Send class="size-3.5 shrink-0" />
                <span class="truncate">{item.followed_up_at ? s.chaseAgain : s.followUp}</span>
              </Button>
            {/if}
          </div>
        </div>
      {/if}
      {#if buddyMember && buddyApplicationId !== null}
        <label class="flex items-center gap-2 text-sm text-muted-foreground">
          <input
            type="checkbox"
            checked={!hiddenInBuddyFeed}
            disabled={buddyVisibilityBusy}
            onchange={toggleBuddyVisibility}
          />
          {s.showInBuddyFeed}
        </label>
      {/if}
      {#if buddyVisibilityError}
        <p class="text-sm text-warning-strong" role="alert">{buddyVisibilityError}</p>
      {/if}

      <div
        bind:this={tabStrip}
        {@attach tabEdges.attach}
        class={cn(
          'no-scrollbar overflow-x-auto',
          !tabEdges.atStart && 'mask-l-from-90%',
          !tabEdges.atEnd && 'mask-r-from-90%',
        )}
      >
        <div
          role="tablist"
          aria-label={s.tabsAria}
          use:tablist={tab}
          class="flex w-max min-w-full items-center gap-3 sm:gap-4"
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
  <div class="min-h-0 flex-1 overflow-y-auto overscroll-contain">
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
              <p class="text-sm font-medium">{s.howDidItClose}</p>
              <div class="flex flex-wrap gap-2">
                {#each CLOSED_OUTCOMES as o (o)}
                  <Button variant="outline" onclick={() => onchooseoutcome(o)}>{humanizeStage(o, locale())}</Button>
                {/each}
              </div>
            </div>
          {/if}

          <label class="flex flex-col gap-1 text-sm">
            <span class="font-medium">{s.stage}</span>
            <select
              value={item.stage ?? ''}
              onchange={(e) => onsetstage(e.currentTarget.value)}
              class="min-h-11 rounded-md border border-input bg-transparent px-2 py-1.5 text-base sm:min-h-0 sm:text-sm"
            >
              <option value="">{s.noStage}</option>
              <!-- Grouped so `Closed` reads as a heading over its three outcomes rather than
                   as a fifth state competing with them — the same four groups the board's
                   columns use, from the same generated table. -->
              {#each groupedStages(locale()) as g (g.id)}
                <optgroup label={g.label}>
                  {#each g.options as s (s.value)}
                    <option value={s.value}>{s.label}</option>
                  {/each}
                </optgroup>
              {/each}
            </select>
          </label>

          <div class="flex flex-col gap-1 text-sm">
            <span class="font-medium">{s.notes}</span>
            <NoteEditor value={item.notes ?? ''} onsave={onsavenotes} label={s.notes} showSaveState />
          </div>
        </div>
      {:else if tab === 'history'}
        {#if !item.job}
          <p class="text-sm text-muted-foreground">{s.historyUnavailable}</p>
        {:else if emailsLoading || emailsError}
          <JobDrawerLoadState loading={emailsLoading} error={emailsError} onretry={() => loadEmails(true)} />
        {:else}
          {#if events.length}
            <div class="flex flex-col gap-1 text-sm">
              <ol class="flex flex-col gap-1.5">
                {#each events as e (e.id)}
                  <!-- The event first and its age beneath it on a phone, where a fixed time
                       column wrapped "11 секунд назад" onto two lines beside a one-line
                       event; side by side from sm up, the age kept whole on the right. -->
                  <li class="flex items-baseline gap-2">
                    <span class="shrink-0 text-xs {eventTone(e.kind)}" aria-hidden="true">●</span>
                    <div class="flex min-w-0 flex-1 flex-col sm:flex-row sm:items-baseline sm:justify-between sm:gap-3">
                      <span class="min-w-0 text-sm">{eventLabel(e, locale())}</span>
                      <time
                        datetime={e.occurred_at}
                        class="text-xs text-muted-foreground sm:shrink-0 sm:whitespace-nowrap"
                      >
                        {timeAgo(e.occurred_at, locale())}
                      </time>
                    </div>
                  </li>
                {/each}
              </ol>
            </div>
          {:else}
            <p class="text-sm text-muted-foreground">{s.noHistory}</p>
          {/if}
        {/if}
      {:else if tab === 'auto_apply' && (emailsLoading || emails === null || emailsError)}
        <JobDrawerLoadState
          loading={emailsLoading || (emails === null && !emailsError)}
          error={emailsError}
          onretry={() => loadEmails(true)}
        />
      {:else if tab === 'auto_apply'}
        <JobDrawerAutoApply
          slug={item.job?.public_slug ?? null}
          {hasPosting}
          bind:autoApply
          onevents={(next) => (events = next)}
          {autoApplied}
          {applyChatError}
          onstartapplychat={startApplyChat}
          {onautoapplyreview}
          ongone={() => (tab = 'application')}
        />
      {:else if tab === 'fit'}
        <div class="flex flex-col gap-6">
          {#if posting}
            <!-- No match analysis handed in, deliberately: `MatchAnalysisFull` directly
                 below IS the analysis, so the compact card the block would draw from it
                 would restate the panel's own content and offer a "View full analysis"
                 link that closes the drawer to show what is already open. -->
            <JobMatch job={posting} matchAnalysis={null} />
            <MatchAnalysisFull job={posting} />
          {:else if item.job}
            <p class="text-sm text-muted-foreground">{s.loading}</p>
          {/if}
        </div>
      {:else if tab === 'emails'}
        <div class="flex flex-col gap-2">
          <!-- The mail says one thing, the stage says another, and nothing moved it. That is
               the rule working — mail never settles an application — so the resolution is
               offered rather than applied, and it goes through the ordinary stage change so
               the ledger records the candidate as its source. -->
          {#if stageSuggestion}
            <div
              class="flex flex-wrap items-center gap-2 rounded-md border border-warning/50 bg-warning-muted/40 px-3 py-2"
            >
              <span class="min-w-0 flex-1 text-sm">
                {format(s.looksLike, {
                  signal: statusLabel(stageSuggestion.signal, locale()).toLowerCase(),
                  stage: item.stage ? humanizeStage(item.stage, locale()) : s.stageUnset,
                })}
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
                {format(s.moveTo, { stage: humanizeStage(stageSuggestion.stage, locale()) })}
              </Button>
              <button
                type="button"
                class="shrink-0 text-xs text-muted-foreground underline-offset-2 hover:underline"
                onclick={() => (stageSuggestion = null)}
              >
                {s.dismiss}
              </button>
            </div>
          {/if}
          <div class="flex flex-wrap items-center gap-2">
            <Button variant="outline" size="sm" href={resolve('/my/inbox')} class="h-9">
              {s.linkEmail}
            </Button>
            <span class="text-xs text-muted-foreground">{s.linkEmailHint}</span>
          </div>
          {#if item.job}
            <JobDrawerMailRecall slug={item.job.public_slug} {company} {canRecall} onlinked={() => loadEmails(true)} />
          {/if}
          {#if emailsLoading}
            <p class="text-sm text-muted-foreground">{s.loadingEmails}</p>
          {:else if emailsError}
            <p class="text-sm text-destructive">{emailsError}</p>
          {:else if !emails || emails.length === 0}
            <p class="text-sm text-muted-foreground">{s.noEmailsLinked}</p>
          {:else}
            <JobDrawerEmailList {emails} />
          {/if}
        </div>
      {:else}
        <div class="flex flex-col gap-5">
          {#if !item.job}
            <!-- The posting was removed from the catalogue. What it said is genuinely
                 gone; saying so is better than an empty tab that reads as a bug. -->
            <p class="text-sm text-muted-foreground">
              {s.postingRemoved}
            </p>
          {:else if posting?.description}
            <JobDescription html={posting.description} />
          {:else if !posting}
            <p class="text-sm text-muted-foreground">{s.loading}</p>
          {:else}
            <p class="text-sm text-muted-foreground">{s.noDescription}</p>
          {/if}

          {#if posting?.skills?.length}
            <div class="flex flex-col gap-2 border-t border-border pt-5">
              <p class={sectionLabel}>{s.skills}</p>
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

  <!-- Pinned footer: remove-from-board (destructive card action). A flex sibling of the
       scrolling body, not an overlay, so it never sits over the notes editor. The bottom
       padding leaves room for both the button and the home indicator. -->
  <div class="shrink-0 border-t border-border">
    <div
      class="mx-auto flex w-full max-w-2xl justify-end px-5 pb-[max(1.25rem,env(safe-area-inset-bottom))] pt-3 sm:px-6"
    >
      <Button
        variant="ghost"
        size="sm"
        onclick={() => (confirmRemoveOpen = true)}
        class="h-11 gap-1.5 text-destructive-strong hover:bg-destructive/10 hover:text-destructive-strong sm:h-8"
      >
        <Trash2 class="size-4" />
        {s.removeFromBoard}
      </Button>
    </div>
  </div>

  <!-- Inside the drawer's root on purpose: the drawer covers the screen at z-50, so a panel
       docked outside it would sit underneath. Here it is drawn over the drawer's right edge,
       or over the whole drawer where there is no room to dock. -->
  {#if applyChatId}
    <AssistantPanel open={applyChatOpen} title={s.applyChat.title} onClose={() => (applyChatOpen = false)}>
      {#snippet children(setSessionId)}
        <AssistantChat
          session={applyChatId ?? undefined}
          showSessionRail={false}
          syncApplication={applyChatOpen}
          onSessionChange={setSessionId}
          onTurnComplete={() => void loadEmails(true)}
        />
      {/snippet}
    </AssistantPanel>
  {/if}
</div>

<!-- Outside the panel's root on purpose: the panel's focus trap handles Tab on its own
     subtree, and the native modal dialog already traps focus by itself. -->
<ConfirmDialog
  bind:open={confirmRemoveOpen}
  title={format(s.removeDialogTitle, { job: jobLabel })}
  description={s.removeDialogDescription}
  confirmLabel={s.removeConfirm}
  cancelLabel={s.removeCancel}
  variant="destructive"
  onConfirm={onremove}
/>
