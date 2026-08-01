<script lang="ts">
  import { resolve } from '$app/paths';
  import { goto } from '$app/navigation';
  import { Button, Badge, cn } from '$lib/ui';
  import { Trash2, X, ExternalLink, Mic, Send, Target, SquarePen } from '@lucide/svelte';
  import { STAGES, humanizeStage } from '$lib/stages';
  import { canFollowUp } from '$lib/followup';
  import { CLOSED_OUTCOMES, type ClosedOutcome } from '$lib/board';
  import { timeAgo, errorMessage } from '$lib/utils';
  import { tablist } from '$lib/actions/tablist';
  import { cardTags } from '$lib/enrichment';
  import CompanyLogo from './CompanyLogo.svelte';
  import MatchAnalysisFull from './MatchAnalysisFull.svelte';
  import JobMatch from './JobMatch.svelte';
  import NoteEditor from './NoteEditor.svelte';
  import { api } from '$lib/api';
  import type { EmailBody } from '$lib/api';
  import { currentUser } from '$lib/auth.svelte';
  import { statusLabel, statusClass } from '$lib/emailStatus';
  import { avatarInitials, avatarColor } from '$lib/avatar';
  import type { MyJob, ApplicationEmail } from '$lib/types';
  import { focusTrap } from '$lib/actions/focusTrap';

  let {
    item,
    pendingOutcome,
    onsetstage,
    onsavenotes,
    onchooseoutcome,
    onremove,
    onclose,
    onrehearse,
    onfollowup,
    rehearsing = false,
    rehearsalError = null,
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
    onfollowup: (item: MyJob) => void;
    // A rehearsal is one round trip before the navigation; the board owns the flag so a
    // second click in that window cannot mint a second conversation.
    rehearsing?: boolean;
    // ...and owns the failure too. It has to be rendered in here: this panel covers the
    // viewport, so a message left behind on the board would be invisible to the person
    // who pressed the button.
    rehearsalError?: string | null;
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
    await goto(resolve('/tailor/[slug]', { slug: item.job.public_slug }));
  }

  // Rehearse, Analyze and Tailor all need the posting. Absent rather than disabled when
  // it is gone — the same treatment View job gets. Follow up does not need one: the chase
  // is addressed to the employer, which the application knows by itself.
  const hasPosting = $derived(!!item.job);
  const offersFollowUp = $derived(canFollowUp(item));

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

  async function loadEmails() {
    if (emails !== null || emailsLoading) return;
    emailsLoading = true;
    emailsError = null;
    try {
      if (!item.job) return;
      const app = await api.getTrackedApplication(item.job.public_slug);
      emails = app.emails;
    } catch (e) {
      emailsError = errorMessage(e, 'Failed to load emails.');
    } finally {
      emailsLoading = false;
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
  let tags = $derived(item.job ? cardTags(item.job) : []);
  let stageLabel = $derived(item.stage ? humanizeStage(item.stage) : null);

  // Interaction timeline shown as a wizard-style stepper up top, in engagement-funnel
  // order (viewed → saved → applied): Applied is the deepest step, so it anchors the
  // right end regardless of the raw last-viewed timestamp.
  let activity = $derived(
    [
      { label: 'Viewed', at: item.viewed_at },
      item.saved_at ? { label: 'Saved', at: item.saved_at } : null,
      item.applied_at ? { label: 'Applied', at: item.applied_at } : null,
    ].filter((x): x is { label: string; at: string } => x !== null),
  );

  // Lock background scroll while the fullscreen panel is open, restored on unmount
  // (close / job switch). A DOM side-effect — the legitimate use of $effect.
  $effect(() => {
    const prev = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.body.style.overflow = prev;
    };
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
          <CompanyLogo name={company} size="size-9" />
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

      {#if activity.length}
        <ol class="flex items-center gap-2">
          {#each activity as a, i (a.label)}
            <li class="flex shrink-0 items-center gap-1.5">
              <span class="size-2 rounded-full bg-brand"></span>
              <span class="whitespace-nowrap text-xs">
                <span class="font-medium text-foreground">{a.label}</span>
                <span class="text-muted-foreground">{timeAgo(a.at)}</span>
              </span>
            </li>
            {#if i < activity.length - 1}
              <li aria-hidden="true" class="h-px min-w-4 flex-1 bg-border"></li>
            {/if}
          {/each}
        </ol>
      {/if}

      <!-- What the candidate can do about this application, on every tab. The card used
           to carry Rehearse and Follow up in a strip beside its badges, which is both
           less room than they need and the reason the card could not be dragged. -->
      {#if hasPosting || offersFollowUp}
        <div class="flex flex-wrap items-center gap-2">
          {#if hasPosting}
            <Button variant="outline" size="sm" onclick={() => onrehearse(item)} disabled={rehearsing} class="gap-1.5">
              <Mic class="size-3.5" />
              {rehearsing ? 'Starting…' : 'Rehearse'}
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
            <!-- The analysis is already in this panel; sending the user to /match would
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
      {#if rehearsalError}
        <p class="text-sm text-warning-strong" role="alert">{rehearsalError}</p>
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

          <label class="flex flex-col gap-1 text-sm">
            <span class="font-medium">Stage</span>
            <select
              value={item.stage ?? ''}
              onchange={(e) => onsetstage(e.currentTarget.value)}
              class="rounded-md border border-input bg-transparent px-2 py-1.5 text-sm"
            >
              <option value="">No stage</option>
              {#each STAGES as s (s.value)}
                <option value={s.value}>{s.label}</option>
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
          {#if item.job}
            <JobMatch job={item.job} />
            <MatchAnalysisFull job={item.job} />
          {/if}
        </div>
      {:else if tab === 'emails'}
        <div class="flex flex-col gap-2">
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
                      <span class="mt-1 inline-block rounded border px-1.5 text-[10px] leading-4 {statusClass(e.status_signal)}">
                        {statusLabel(e.status_signal)}
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
          {:else if item.job.description}
            <!-- Description is server-sanitized HTML (see internal/sources), safe to render. -->
            <!-- eslint-disable-next-line svelte/no-at-html-tags -- server-sanitized; the rule flags every {@html} regardless -->
            <div class="job-description text-sm leading-relaxed">{@html item.job.description}</div>
          {:else}
            <p class="text-sm text-muted-foreground">No description available.</p>
          {/if}

          {#if item.job?.skills?.length}
            <div class="flex flex-col gap-2 border-t border-border pt-5">
              <p class={sectionLabel}>Skills</p>
              <div class="flex flex-wrap gap-1.5">
                {#each item.job.skills as skill (skill)}
                  <Badge variant="brand">{skill}</Badge>
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

<style>
  /* Tab rail scrolls horizontally on narrow screens without a visible scrollbar. */
  .no-scrollbar {
    scrollbar-width: none;
    -ms-overflow-style: none;
  }
  .no-scrollbar::-webkit-scrollbar {
    display: none;
  }

  /* Descriptions are arbitrary scraped HTML: a long URL — or words glued by
     non-breaking spaces — must wrap instead of forcing a horizontal scroll.
     Styles mirror JobView's .job-description so the read matches the job page. */
  .job-description {
    overflow-wrap: break-word;
  }

  .job-description :global(h1),
  .job-description :global(h2),
  .job-description :global(h3),
  .job-description :global(h4) {
    margin-top: 1.25rem;
    margin-bottom: 0.5rem;
    font-weight: 600;
  }

  .job-description :global(p) {
    margin: 0.5rem 0;
  }

  .job-description :global(ul),
  .job-description :global(ol) {
    margin: 0.5rem 0;
    padding-left: 1.25rem;
  }

  .job-description :global(li) {
    display: list-item;
    list-style: disc outside;
    margin: 0.25rem 0;
  }

  /* ATS boards (e.g. Greenhouse) wrap each <li> in a block <p>; collapse its
     margins so the bullet sits beside the text instead of on its own line. */
  .job-description :global(li) > :global(p) {
    margin: 0;
  }

  .job-description :global(a) {
    text-decoration: underline;
  }

  .job-description :global(b),
  .job-description :global(strong) {
    font-weight: 600;
  }
</style>
