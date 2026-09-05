<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { ArrowRight, Bookmark, Check, CheckCircle2, Eye, Flag, MessageSquare } from '@lucide/svelte';
  import { ApiError, api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { autoApplyButtonState } from '$lib/autoApplyButton';
  import { onboardingUrl } from '$lib/onboardingGate.svelte';
  import { promptSignIn } from '$lib/signin';
  import { filterHref, formatSalary, requirementGroups, summaryFacets } from '$lib/enrichment';
  import { freshnessBadges } from '$lib/freshness';
  import { markViewed } from '$lib/viewedJobs.svelte';
  import { markSaved, markUnsaved } from '$lib/savedJobs.svelte';
  import { track } from '$lib/analytics';
  import { foreignContentLang } from '$lib/seo';
  import type { Display } from '$lib/generated/contracts';
  import type { Job, UserJob } from '$lib/types';
  import { companyLogoUrl } from '$lib/logo';
  import { Badge, Button, Chip, EntityLogo, TabStrip, tabStripId } from '$lib/ui';
  import { formatDate, formatDateOrAgo, formatDateTime } from '$lib/utils';
  import AddToListButton from './AddToListButton.svelte';
  import AdzunaAttribution from './AdzunaAttribution.svelte';
  import BackerBadge from './BackerBadge.svelte';
  import CountryFlagStack from './CountryFlagStack.svelte';
  import JobApplyForm, { applyFormWorthShowing } from './JobApplyForm.svelte';
  import JobCompanyPanel from './JobCompanyPanel.svelte';
  import JobDescription from './JobDescription.svelte';
  import JobMatch from './JobMatch.svelte';
  import { supersedesReality } from '$lib/ghost';
  import GhostChecklist from './GhostChecklist.svelte';
  import RealityBadge from './RealityBadge.svelte';
  import ReferralBlock from './ReferralBlock.svelte';
  import ReportDialog from './ReportDialog.svelte';
  import SkillChip from './SkillChip.svelte';
  import VoteControl from './VoteControl.svelte';

  // The job is server-rendered: it arrives as a prop from the route's `load`, so
  // the article's content is in the initial HTML. Only the per-user interactions
  // below hydrate client-side.
  // `applyForm` is the employer's own screening form when its ATS publishes one —
  // null for most postings, which is the ordinary case and simply means one fewer tab.
  let { job, applyForm = null }: { job: Job; applyForm?: Display | null } = $props();

  // Set on the two subtrees below that carry the posting's own words; undefined
  // (so unset) when it is English. See foreignContentLang for why the document
  // stays `en` regardless.
  const contentLang = $derived(foreignContentLang(job));

  // The signed-in user's interaction with this job (null when signed out or not
  // yet loaded). `showApplyPrompt` is the post-click "Did you apply?" question.
  let interaction = $state.raw<UserJob | null>(null);
  let showApplyPrompt = $state(false);
  // Open-thread count for the "Discussion · N" badge; loaded client-side so the
  // page renders immediately and the number fills in. Failures leave it hidden.
  let threadCount = $state<number | null>(null);
  $effect(() => {
    let alive = true;
    api
      .countThreads('job', job.public_slug)
      .then((n) => alive && (threadCount = n))
      .catch(() => {});
    return () => {
      alive = false;
    };
  });
  // Set after the user confirms "Yes" on the apply prompt: surfaces a one-tap
  // link to the Tracking board where the job now sits. Reset when the job changes.
  let justApplied = $state(false);
  // Signed-out gate: the "Apply" click offers sign-in before opening the posting.
  let showSignInPrompt = $state(false);
  // The report dialog (a problem-with-this-job complaint) opens over the page.
  let showReport = $state(false);
  const applied = $derived(interaction?.applied_at != null);
  const saved = $derived(interaction?.saved_at != null);

  // Presentational values derived from the (server-rendered) job.
  // Both read as an age for their first day ("20 minutes ago") and as a date after it —
  // the same label the feed's card already gives a posting, so a reader arriving from
  // the list meets the answer in the form they just left.
  const posted = $derived(formatDateOrAgo(job.posted_at));
  // When the posting's own content last changed. `jobs.updated_at` is deliberately left
  // unstamped by the liveness refresh (internal/platform/db/queries/jobs.sql), so the column
  // means "the words moved", not "the crawler came back" — which is the only reading that
  // earns a line beside the posting date.
  const updated = $derived(formatDateOrAgo(job.updated_at));
  const e = $derived(job.enrichment ?? {});
  const salary = $derived(formatSalary(e));
  const facets = $derived(summaryFacets(job));
  // What the posting itself asks for, grouped required-then-preferred. Empty for a
  // job in which neither the model nor the description parser found requirements,
  // which is what makes the section disappear rather than head an empty list.
  const requirements = $derived(requirementGroups(e.requirements));
  // Engagement counters (distinct signed-in viewers / applicants), served on the
  // job. A zero metric is omitted so the line never reads as a dead "0 views".
  const views = $derived(job.view_count ?? 0);
  const applies = $derived(job.applied_count ?? 0);
  // "New" / "Be an early applicant" — see freshness.ts for why the posting date alone
  // does not earn either.
  const freshness = $derived(freshnessBadges(job.posted_at, job.reality, applies, job.closed_at));

  // The content column is tabbed so "who are these people?" and "what will they ask
  // me?" are answerable without leaving the posting. Page state, not a route: the
  // company copy already has a canonical home at /companies/<slug>, and a second URL
  // serving it would be a thin duplicate we'd then have to keep out of the index.
  //
  // Built per job rather than fixed, because neither of the two extra tabs is always
  // there: a posting can arrive without a company slug, and only a few ATS platforms
  // publish a form we can read. A tab is offered only when its panel has something in
  // it — `applyFormWorthShowing` is the same predicate the panel itself renders on, so
  // the two cannot disagree about whether there is anything to show.
  type ContentTab = 'description' | 'company' | 'application';

  const companySlug = $derived(job.company_slug ?? '');
  const contentTabs: { id: ContentTab; label: string }[] = $derived([
    { id: 'description', label: 'Description' },
    ...(companySlug ? [{ id: 'company' as const, label: 'Company' }] : []),
    ...(applyFormWorthShowing(applyForm)
      ? [{ id: 'application' as const, label: 'Application' }]
      : []),
  ]);
  let contentTab = $state<ContentTab>('description');
  // Per-instance so a second JobView on one page can't claim the same panel, which
  // would leave both strips' aria-controls pointing at the first one's panel.
  const panelId = $props.id();
  // Reset when the visitor navigates client-side to another job: JobView is not
  // remounted on a param change, so the Company tab would otherwise stay selected
  // over a company they never asked about.
  $effect(() => {
    void job.public_slug;
    contentTab = 'description';
  });

  // The pinned header. Once the posting's own header has scrolled under the top bar,
  // a one-line copy of it — company, title, apply — takes its place, so "who is this
  // for?" and the button stay a glance away through a description that routinely runs
  // several screens.
  //
  // It rides a zero-height sticky rail inside an absolutely-positioned overlay, so the
  // bar occupies no space in the flow and appearing can never shift the text under it.
  // The obvious alternative — pinning the real header and collapsing it — cannot say
  // that: its flow box loses ~90px the instant it pins, jerking the paragraph the
  // reader is mid-sentence in, and again in reverse on the way back up.
  const PINNED_HEADER_TOP = 56; // `top-14` on the rail below, and the top bar's own `h-14`.
  let headerSentinel: HTMLElement | undefined = $state();
  let headerPinned = $state(false);
  $effect(() => {
    const el = headerSentinel;
    if (!el) return;
    const io = new IntersectionObserver(
      (entries) => {
        const entry = entries[0];
        if (!entry) return;
        // `isIntersecting` alone also reads false while the sentinel is BELOW the fold,
        // which is where it sits on a short viewport before anything has been scrolled.
        // The rect settles which edge it left by.
        headerPinned =
          !entry.isIntersecting && entry.boundingClientRect.top < PINNED_HEADER_TOP;
      },
      { rootMargin: `-${PINNED_HEADER_TOP}px 0px 0px 0px` },
    );
    io.observe(el);
    return () => io.disconnect();
  });

  // Funnel view — captured for everyone (unlike the authed-only server record
  // below). Keyed on the slug alone so it fires once per job and never re-emits
  // when an unrelated dependency (e.g. auth state) changes mid-view.
  $effect(() => {
    const slug = job.public_slug;
    track('job_view', { slug, source: job.source });
  });

  // Record a view for signed-in users once the page hydrates (browser only).
  // Silent history that also tells us whether they
  // already applied; a failed view must not break the page. Re-runs on client
  // navigation to another job, resetting the per-user state first.
  $effect(() => {
    const slug = job.public_slug; // track the current job
    interaction = null;
    showApplyPrompt = false;
    justApplied = false;
    showSignInPrompt = false;
    if (!isAuthenticated()) return; // effects run client-only, so no browser guard needed
    api.recordJobView(slug)
      .then((rec) => {
        // Mark it locally so its card dims on back-navigation without a reload.
        markViewed(slug);
        if (job.public_slug === slug) interaction = rec;
      })
      .catch(() => {});
  });

  // The Apply link opens the external posting; once the user has gone to apply,
  // offer the "Did you apply?" choice (only when signed in and not already applied).
  // Signed-out visitors are gated first: the click is intercepted (the link does
  // not open) and a sign-in offer is shown instead — the posting opens only via
  // "View without signing in" below.
  function onApplyClick(e: MouseEvent) {
    // Apply-intent — fired regardless of auth (the CTA click is the funnel step).
    track('job_apply', { slug: job.public_slug, source: job.source });
    if (!isAuthenticated()) {
      e.preventDefault();
      showSignInPrompt = true;
      return;
    }
    if (!applied) showApplyPrompt = true;
  }

  // Gate — "Sign up" routes to /onboarding, which bounces an anonymous visitor to
  // /signin to register before continuing into the CV/profile wizard; "View without
  // signing in" opens the posting in a new tab (the navigation the click was holding
  // back).
  function signUpFromGate() {
    showSignInPrompt = false;
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- onboardingUrl() wraps resolve('/onboarding'); the rule can't see through the appended ?returnTo= query
    void goto(onboardingUrl(page.url.pathname + page.url.search));
  }

  function viewWithoutSignIn() {
    showSignInPrompt = false;
    window.open(job.url, '_blank', 'noopener,noreferrer');
  }

  async function confirmApplied() {
    try {
      interaction = await api.markJobApplied(job.public_slug);
    } catch {
      // Leave the prompt up so the user can retry; nothing else to do.
      return;
    }
    showApplyPrompt = false;
    justApplied = true; // offer the board link now that the job is tracked
    track('job_track', { slug: job.public_slug, stage: 'applied' });
  }

  // "No": purely local — the job must not enter the tracker.
  function dismissApplyPrompt() {
    showApplyPrompt = false;
  }

  // Saving requires an account.
  function onSaveClick() {
    if (!isAuthenticated()) {
      promptSignIn();
      return;
    }
    toggleSave();
  }

  // Reporting also requires an account (the report is attributed to the user).
  function onReportClick() {
    if (!isAuthenticated()) {
      promptSignIn();
      return;
    }
    showReport = true;
  }

  // The discussion is members-only: a signed-out click is held back (the link
  // does not navigate) and the sign-in page opens instead. The href stays put
  // so the route is still SSR-linkable for signed-in visitors.
  function onDiscussionClick(e: MouseEvent) {
    if (!isAuthenticated()) {
      e.preventDefault();
      promptSignIn();
    }
  }

  // The toggle flips on the server's answer, not optimistically: both endpoints
  // return the full interaction, so the button can never drift from the truth.
  async function toggleSave() {
    try {
      interaction = saved ? await api.unsaveJob(job.public_slug) : await api.saveJob(job.public_slug);
      // Keep the shared saved set in sync so a browse card's bookmark reflects this
      // on back-navigation without a reload (mirrors markViewed above).
      if (interaction?.saved_at != null) {
        markSaved(job.public_slug);
        track('job_save', { slug: job.public_slug });
      } else markUnsaved(job.public_slug);
    } catch {
      // Leave the current state; the user can retry.
    }
  }

  // Auto-apply (openspec/changes/auto-apply-submit-trigger): PRO-only, Greenhouse-only.
  // Eligibility (plan tier, base CV) is not known client-side — the button stays
  // clickable and the backend's own 402/409 message is what tells an ineligible
  // caller why, surfaced below rather than pre-empted here.
  let autoApplyOverrideStatus = $state<string | null>(null);
  let autoApplySubmitting = $state(false);
  let autoApplyError = $state<string | null>(null);
  const autoApplyState = $derived(
    autoApplyButtonState(job.source, autoApplyOverrideStatus ?? job.auto_apply_status, applied),
  );

  async function onAutoApplyClick() {
    if (!isAuthenticated()) {
      promptSignIn();
      return;
    }
    autoApplyError = null;
    autoApplySubmitting = true;
    try {
      await api.autoApplyJob(job.public_slug);
      autoApplyOverrideStatus = 'queued';
      track('job_auto_apply', { slug: job.public_slug });
    } catch (err) {
      autoApplyError = err instanceof ApiError ? err.message : 'Something went wrong — please try again.';
    } finally {
      autoApplySubmitting = false;
    }
  }
</script>

<!-- The apply CTA renders twice: inline in the header on desktop, and in the
     mobile sticky bar at the end of the article. Sole difference is size + layout
     classes, so both share this snippet.
     nofollow: the destination is the posting's own site, which the catalogue never
     vetted — the same stance the description sanitizer takes on in-body links
     (internal/sources/sanitize.go). Without it a submitted vacancy buys a followed
     link from every job page, which is what the SEO submissions are actually after. -->
{#snippet applyCta(size: 'md' | 'lg', className: string)}
  <Button
    variant="primary"
    {size}
    href={job.url}
    target="_blank"
    rel="nofollow noopener noreferrer"
    onclick={onApplyClick}
    class={className}
  >
    Apply <ArrowRight class="size-4" />
  </Button>
{/snippet}

<!-- Auto-apply (openspec/changes/auto-apply-submit-trigger): beside Apply, not a
     replacement for it — auto-apply still goes through the same ATS in the end, this
     button only starts the tailor-then-review sequence. Absent entirely off
     autoApplyButtonState's `hidden` (any source but Greenhouse today). `idle` is the only
     clickable state; `queued`/`declined`/`applied`/`failed` render disabled (the
     `disabled:opacity-50` the button variant already carries) so a caller who already has
     an attempt, already applied for real, or whose attempt cmd/auto-apply gave up on, sees
     that at a glance rather than clicking into a 200 or a 409 that changes nothing. -->
{#snippet autoApplyCta(className: string)}
  {#if autoApplyState.kind !== 'hidden'}
    <Button
      variant="secondary"
      size="md"
      disabled={autoApplyState.kind !== 'idle' || autoApplySubmitting}
      onclick={onAutoApplyClick}
      class={className}
    >
      {#if autoApplyState.kind === 'queued'}
        Auto-apply queued
      {:else if autoApplyState.kind === 'declined'}
        Auto-apply declined
      {:else if autoApplyState.kind === 'applied'}
        Already applied
      {:else if autoApplyState.kind === 'failed'}
        Auto-apply couldn't complete
      {:else}
        Auto-apply
      {/if}
    </Button>
  {/if}
{/snippet}

<!-- Save, a quiet peer of the apply CTA rather than the full-width button it was in the
     sidebar: it belongs beside "Apply", because keeping a job and opening it are the two
     things a reader does with one. The filled bookmark is the state.
     `iconOnly` is for the pinned header, whose one line already holds the company, the
     title and the CTA; aria-label and the tooltip name the button either way. -->
{#snippet saveButton(className = '', iconOnly = false)}
  <Button
    variant="ghost"
    size="sm"
    onclick={onSaveClick}
    aria-pressed={saved}
    aria-label={saved ? 'Remove from saved' : 'Save job'}
    title={saved ? 'Saved' : 'Save'}
    class={`shrink-0 gap-1.5 px-2 ${saved ? 'text-brand hover:text-brand' : 'text-muted-foreground'} ${className}`}
  >
    <Bookmark class="size-4 shrink-0 {saved ? 'fill-current' : ''}" aria-hidden="true" />
    {#if !iconOnly}{saved ? 'Saved' : 'Save'}{/if}
  </Button>
{/snippet}

<!-- Report, the same shape as Save and, like it, `ghost`: complaining about a posting is
     the rarest thing on the strip, and a filled or outlined box would give it the
     standing of the apply CTA beside it. -->
{#snippet reportButton()}
  <Button
    variant="ghost"
    size="sm"
    onclick={onReportClick}
    aria-label="Report this job"
    title="Report this job"
    class="shrink-0 gap-1.5 px-2 text-muted-foreground"
  >
    <Flag class="size-4 shrink-0" aria-hidden="true" />
    Report
  </Button>
{/snippet}

<!-- When the posting went up and when its words last moved. Two facts ABOUT the posting,
     so they ride the provenance line with the company and the badges rather than the
     sidebar's source row, where a reader comparing freshness had to look past the match
     score and the salary to find them.
     "Updated" is dropped when it renders to the same label as the posting date: a job
     written once and never touched would otherwise say the same thing twice. Comparing
     the LABELS rather than the timestamps is the point — two edits an hour apart on one
     day are a real difference while both read as ages, and stop being one the day they
     both collapse into that date.
     The exact clock time rides the `title` either way, since a reader who has read "2
     hours ago" off a server-rendered page may want to know how old the page is.
     Rendered twice, one visible at a time (the caller passes the display class): to the
     right of the provenance line from lg, and under the title below it, where that line is
     a single non-wrapping row whose company name is already truncating to fit. -->
{#snippet postingDates(className: string)}
  {#if posted}
    <div class={`items-center gap-x-2 text-xs text-muted-foreground ${className}`}>
      <span class="whitespace-nowrap">
        Posted <time datetime={job.posted_at} title={formatDateTime(job.posted_at)}>{posted}</time>
      </span>
      {#if updated && updated !== posted}
        <span aria-hidden="true">·</span>
        <span class="whitespace-nowrap">
          Updated <time datetime={job.updated_at} title={formatDateTime(job.updated_at)}>{updated}</time>
        </span>
      {/if}
    </div>
  {/if}
{/snippet}

<!-- The action strip: everything a reader does WITH the posting — talk about it, flag
     it, keep it, open it — on one line, sharing the tab row's rule. It carries the
     rule itself (`border-b`) rather than sitting above one, so the line reads as a
     single edge across the column: the strip and the TabStrip beside it are aligned on
     their bottoms, and each draws its own half of it.
     Rendered in two places, one visible at a time (the caller passes the display class):
     the tab row on lg, and directly under the title below it, where the sidebar stacks
     between the title and the description and a strip left on the tab row would put Save
     a whole screen away from the job it saves. Only the apply CTA drops out below lg — the sticky
     bottom bar carries it there, which is also what leaves the phone room for the labels. -->
{#snippet actionStrip(className: string)}
  <div class={`shrink-0 items-center gap-1.5 ${className}`}>
    <a
      class="inline-flex shrink-0 items-center gap-1.5 px-1 text-sm font-medium text-primary hover:underline"
      href={resolve('/jobs/[slug]/discussion', { slug: job.public_slug })}
      onclick={onDiscussionClick}
    >
      <MessageSquare class="size-4 shrink-0" aria-hidden="true" />
      Discussion{threadCount ? ` · ${threadCount}` : ''}
    </a>
    {@render reportButton()}
    {@render saveButton()}
    <AddToListButton jobSlug={job.public_slug} />
    {@render autoApplyCta('ml-1 hidden shrink-0 lg:inline-flex')}
    {@render applyCta('md', 'ml-1 hidden shrink-0 lg:inline-flex')}
  </div>
{/snippet}

<!-- The posting itself. A snippet because it is rendered either inside the Description
     tab panel or, on a job whose employer we don't know, straight into the column with
     no tab strip above it at all. -->
{#snippet descriptionContent()}
  {#if e.summary}
    <!-- Model-written synopsis (only on enriched jobs). Plain text, capped at
         400 chars server-side — the headline above the full description. -->
    <section class="flex flex-col gap-2">
      <h2 class="text-base font-semibold">Summary</h2>
      <p class="text-sm leading-relaxed text-muted-foreground">{e.summary}</p>
    </section>
  {/if}

  <!-- The Summary above is an LLM synopsis the enrichment prompt pins to English
       (internal/enrich), so the body is the only part of this snippet that takes
       the posting's language. -->
  <JobDescription html={job.description} lang={contentLang} />

  {#if requirements.length}
    <!-- What the posting asks for, lifted from the posting itself. It sits here and
         not in the sidebar because the entries are sentences (~70 chars on average,
         ~9 of them): in the 20rem rail they wrap to roughly fifteen lines and the
         sticky card outgrows the page. Unlike the Summary above, the text is the
         employer's own, so it takes `contentLang` like the description body. -->
    <section class="flex flex-col gap-3 border-t border-border pt-4">
      <h2 class="text-base font-semibold">What they ask for</h2>
      {#each requirements as group (group.priority)}
        <div class="flex flex-col gap-1.5">
          <h3 class="text-sm font-medium text-muted-foreground">{group.label}</h3>
          <!-- Keyed by index, not by text: two entries of a posting can read the
               same, and a duplicate key silently breaks the whole block. The list
               never reorders, so the index is stable. -->
          <ul class="flex list-disc flex-col gap-1 pl-5 text-sm leading-relaxed">
            {#each group.items as requirement, i (i)}
              <li lang={contentLang}>{requirement.text}</li>
            {/each}
          </ul>
        </div>
      {/each}
    </section>
  {/if}

  {#if job.skills?.length}
    <!-- Top-level `skills` is the served (deterministic-dictionary) facet; the raw
         `enrichment.skills` is kept in the JSONB and never served (see JobRow's same
         fix), so reading it here always rendered nothing. -->
    <section class="flex flex-col gap-2 border-t border-border pt-4">
      <h2 class="text-base font-semibold">Skills</h2>
      <ul class="flex flex-wrap gap-1.5">
        {#each job.skills as skill (skill)}
          <li>
            <!-- The chip used to print the raw slug, so a posting read "ci-cd" beside a
                 filter panel reading "CI/CD" — the same skill spelled two ways on one
                 screen. SkillChip labels it from the dictionary and, for a skill the
                 glossary has reached, carries the definition too. -->
            <SkillChip slug={skill} />
          </li>
        {/each}
      </ul>
    </section>
  {/if}
{/snippet}

<!-- Wide layout mirroring /jobs. The company line spans the very top; below it a
     sticky left sidebar (match + salary + metadata) starts level with the title,
     and the description reads in the right column. On mobile everything stacks:
     company + actions → title → metadata → description. -->
<!-- Explicit rows: company + header sized to content, the content row flexible so
     when the sticky sidebar (which spans all three rows) is taller than the right
     column, the slack collects below the content instead of being spread as gaps
     between company/header/summary. -->
<article
  class="flex flex-col gap-4 lg:grid lg:grid-cols-[20rem_minmax(0,1fr)] lg:grid-rows-[auto_auto_minmax(0,1fr)] lg:gap-x-6 lg:gap-y-4"
>
  <!-- One line on a phone, wrapping only from lg: a badge alone on its own row reads as
       a second heading rather than as a note about the line above it. What gives instead
       is the company name — it alone carries `min-w-0`, so it ellipses to make room, and
       the page title, the logo's alt and the pinned bar all still spell it out in full.
       The badges need their own `shrink-0` group rather than the default `min-width:
       auto`, whose floor for text is the longest WORD: a squeezed chip does not stay one
       line, it wraps "Be an early / applicant" inside itself and grows taller than the
       row. -->
  <div
    class="flex min-w-0 items-center gap-x-2 gap-y-2 max-lg:overflow-hidden lg:col-start-2 lg:row-start-1 lg:flex-wrap lg:gap-x-3"
  >
    <EntityLogo
      name={job.company || 'Unknown company'}
      src={companyLogoUrl(job.company) ?? undefined}
      shape="square"
      size="sm"
    />
    <p class="min-w-0 text-sm text-muted-foreground max-lg:truncate">
      {#if job.company_slug}
        <a href={resolve('/companies/[slug]', { slug: job.company_slug })} class="hover:text-foreground hover:underline">
          {job.company || 'Unknown company'}
        </a>
      {:else}
        {job.company || 'Unknown company'}
      {/if}
    </p>

    <div class="flex shrink-0 items-center gap-x-2 gap-y-2 lg:flex-wrap lg:gap-x-3">
      <!-- Who backed the employer. The page has room the feed card does not, so the
           badge carries the brand name too and links to that collection's roles. -->
      <BackerBadge collections={job.collections} withLabel />

      <!-- How long this has really been open, on the provenance line rather than under
           the title: it is a fact ABOUT the posting, like the company and the backer,
           and a chip on its own row read as a headline. The ghost checklist stays below
           the title — it is a disclosure with a criteria list inside, not a chip, and it
           supersedes this badge rather than joining it. -->
      {#if !supersedesReality(job.ghost)}
        <RealityBadge reality={job.reality} postedAt={job.posted_at} detailed />
      {/if}

      <!-- Freshness rides the same provenance line: like the backer and the reality
           badge it describes the POSTING, not the role, so the title keeps a single
           voice and the reader still meets all of it in one glance. A closed posting
           earns nothing here — that rule lives in freshnessBadges, beside the reality
           gate, rather than as a second copy in this template. -->
      {#each freshness as badge (badge.label)}
        <!-- The tooltip rides a wrapper, not the Chip: the primitive takes only
             variant/class/children, so a `title` passed to it would be dropped
             silently and the badge would state a claim with its evidence gone. -->
        <span title={badge.tooltip} class="inline-flex">
          <Chip variant="brand" class="font-semibold">{badge.label}</Chip>
        </span>
      {/each}
    </div>

    <!-- `ml-auto` rather than a grid column: the badges above are a variable-width group,
         so the dates take whatever is left of the line and sit against its right edge. -->
    {@render postingDates('ml-auto hidden shrink-0 lg:flex')}
  </div>

  <header class="flex flex-col gap-3 lg:col-start-2 lg:row-start-2">
    <div class="flex flex-wrap items-center gap-2.5">
      <h1 class="text-2xl font-semibold tracking-tight" lang={contentLang}>{job.title}</h1>
      <!-- Applied stays with the title: it is a fact about THIS READER, not about the
           posting, so it does not belong on the provenance line above. -->
      {#if applied}
        <Chip variant="brand" class="gap-1.5 border-brand/30 font-semibold">
          <CheckCircle2 class="size-3.5" aria-hidden="true" /> Applied
        </Chip>
      {/if}
    </div>

    <!-- Below lg the provenance line is a single non-wrapping row already truncating the
         company name, so the dates read here instead — a caption under the title rather
         than right-aligned, which on one phone-width column would only look adrift. -->
    {@render postingDates('flex lg:hidden')}

    <!-- Below lg the strip rides here rather than on the tab row: the sidebar stacks
         between the title and the description on a phone, so a strip left down there
         would put Save a full screen of match card and metadata away from the job it
         saves. Under the title and not above it — these are the quiet things a reader
         does WITH a posting, and above the title they wedged three tertiary controls
         between the employer and the role. Right-aligned like the lg copy beside the
         tabs, so the strip reads the same on both; `-mr-2` cancels the last button's
         own padding, lining its right edge up with the column's. -->
    {@render actionStrip('-mr-2 flex w-full justify-end border-b border-border pb-2 lg:hidden')}

    <!-- The ghost row supersedes the reality chip (see JobRow). It states the signal
         once for the whole page: a gauge and the hedged wording here, the criteria and
         a link to the full explanation behind its own disclosure. Placed above the
         description, so somebody deciding whether this is worth an hour of unpaid work
         meets the hedge before the pitch rather than after they have already invested
         the reading. The caveat itself now lives on /features/ghost-jobs — the row
         carries the ceiling on the claim ("possibly", two of four), not the essay. -->
    {#if supersedesReality(job.ghost)}
      <GhostChecklist ghost={job.ghost} />
    {/if}

    {#if job.referral_available && job.company_slug}
      <ReferralBlock companySlug={job.company_slug} companyName={job.company} />
    {/if}
  </header>

  <aside class="w-full shrink-0 lg:col-start-1 lg:row-span-3 lg:row-start-1">
    <!-- `top-20`, not `top-6`: the site header is `sticky top-0 h-14` and opaque, so a
         card pinned 24px from the viewport spends the whole read with its first 32px —
         the border, the padding, and the top of the match score — behind it. 80px is
         the header's 56 plus the same 24 of air the card was asking for. Same offset
         DocsNav's rail already uses. -->
    <div class="sticky top-20 flex flex-col gap-4 rounded-xl border border-border bg-card p-4">
      <JobMatch {job} />

      {#if salary}
        <p
          class="border-t border-border pt-4 text-xl font-semibold tabular-nums tracking-tight first:border-t-0 first:pt-0"
        >
          {salary}
        </p>
      {/if}

      {#if facets.length}
        <dl class="flex flex-col gap-2 border-t border-border pt-4 text-sm first:border-t-0 first:pt-0">
          {#each facets as facet (facet.label)}
            <div class="flex items-baseline justify-between gap-3">
              <dt class="shrink-0 text-muted-foreground">{facet.label}</dt>
              {#if facet.values.every((v) => v.flag)}
                <!-- Flag facet (Country): an overlapping cluster of round flags,
                     right-aligned, each linking to its filter. The stack laps the
                     flags over one another so a many-country remote role stays a
                     single compact row instead of wrapping. -->
                <dd class="flex min-w-0 justify-end text-base">
                  <!-- `labelClass` matches the sibling `dd` below (text-sm font-medium,
                       ordinary foreground) — the component's own default is a muted
                       caption sized for the browse card and company panel instead. -->
                  <CountryFlagStack
                    codes={facet.values.map((v) => v.flag ?? '')}
                    link
                    labelClass="text-sm font-medium"
                  />
                </dd>
              {:else}
                <dd class="min-w-0 break-words text-right font-medium"
                  >{#each facet.values as v, i (v.text)}{#if i > 0}, {/if}{#if v.href}<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- internal filter link from filterHref; query-only, no route to resolve --><a
                        href={v.href}
                        class="hover:text-foreground hover:underline">{v.text}</a
                      >{:else}{v.text}{/if}{/each}</dd
                >
              {/if}
            </div>
          {/each}
        </dl>
      {/if}

      <div class="flex flex-col gap-2 border-t border-border pt-4 first:border-t-0 first:pt-0">
        <div class="flex flex-wrap items-center justify-center gap-x-3 gap-y-1.5 text-xs text-muted-foreground">
<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- internal /jobs filter link from filterHref; query-only, no route to resolve -->
          <a href={filterHref('source', job.source)} class="inline-flex">
            <Badge variant="outline" class="transition-colors hover:bg-accent hover:text-foreground">
              {job.source}
            </Badge>
          </a>
          {#if job.source === 'adzuna'}
            <!-- Required by Adzuna's API terms, not a courtesy credit — see the component. It
                 sits in the provenance row beside the source chip, which is where a reader
                 already looks to find out where a posting came from. -->
            <AdzunaAttribution jobUrl={job.url} />
          {/if}
          {#if job.manually_added}
            <Badge variant="secondary">Manually added</Badge>
          {/if}
        </div>
        {#if views > 0 || applies > 0}
          <div class="flex flex-wrap items-center justify-center gap-3 text-xs leading-none text-muted-foreground">
            {#if views > 0}
              <span class="inline-flex items-center gap-1"><Eye class="size-3.5 shrink-0" />{views} {views === 1 ? 'view' : 'views'}</span>
            {/if}
            {#if applies > 0}
              <span class="inline-flex items-center gap-1"><Check class="size-3.5 shrink-0" />{applies} applied</span>
            {/if}
          </div>
        {/if}
      </div>

      <div class="border-t border-border pt-4 first:border-t-0 first:pt-0">
        <div class="flex justify-center">
          <!-- Keyed on the slug so a client-side navigation to another job remounts
               the control and re-seeds its counts/highlight (JobView itself is not
               remounted on param change). -->
          {#key job.public_slug}
            <VoteControl
              target="job"
              slug={job.public_slug}
              upvoteCount={job.upvote_count ?? 0}
              downvoteCount={job.downvote_count ?? 0}
              myVote={job.my_vote ?? 0}
            />
          {/key}
        </div>
      </div>
    </div>
  </aside>

  <div class="relative flex min-w-0 flex-col gap-6 lg:col-start-2 lg:row-start-3">
    <!-- The pinned header (see PINNED_HEADER_TOP). The overlay spans this column, which
         is what gives the rail inside it something to travel: a `sticky` element is
         clamped to its containing block, and the column is exactly the stretch the bar
         should stay pinned over — it releases at the end of the posting, not over the
         related jobs below. The `bottom-14` inset is the bar's own height: the rail is
         zero-height, so without it the bar hangs a further ~52px past the end of the
         overlay and spills over the "See also" strip on the last screen.
         `pointer-events-none` on the overlay keeps the description selectable through
         it; the bar itself takes them back.
         `invisible` rather than a conditional block: keeping the bar mounted lets it
         fade, and visibility:hidden takes it out of the focus order and the a11y tree
         while it is away, so the duplicate apply button is not tabbable from the top
         of the page. -->
    <div class="pointer-events-none absolute inset-x-0 top-0 bottom-14">
      <div bind:this={headerSentinel} aria-hidden="true" class="h-px w-full"></div>
      <div class="sticky top-14 z-20 h-0">
        <div
          class={[
            // Opaque, not frosted: the description slides directly under this and a
            // translucent bar leaves a blurred ghost of the text inside it, which reads
            // as a rendering fault rather than as glass. The mobile apply bar below can
            // afford the frost because nothing scrolls under it at speed.
            'pointer-events-auto flex items-center gap-3 border-b border-border bg-background py-2.5 transition-opacity duration-150',
            headerPinned ? 'opacity-100' : 'invisible opacity-0',
          ]}
        >
          <EntityLogo
            name={job.company || 'Unknown company'}
            src={companyLogoUrl(job.company) ?? undefined}
            shape="square"
            size="sm"
          />
          <p class="min-w-0 flex-1 truncate text-sm">
            {#if job.company_slug}
              <a
                href={resolve('/companies/[slug]', { slug: job.company_slug })}
                class="text-muted-foreground hover:text-foreground hover:underline"
              >
                {job.company || 'Unknown company'}
              </a>
            {:else}
              <span class="text-muted-foreground">{job.company || 'Unknown company'}</span>
            {/if}
            <span class="px-1 text-muted-foreground" aria-hidden="true">·</span>
            <span class="font-semibold" lang={contentLang}>{job.title}</span>
          </p>
          <!-- Hidden below lg for the same reason as the action strip's own copy: on
               mobile the CTA is the sticky bottom bar, and two pinned buttons would
               fight. Save comes along, since this bar is what a reader has in front of
               them for most of a description several screens long. -->
          <div class="hidden shrink-0 items-center gap-2 lg:flex">
            {@render saveButton('size-9 rounded-md px-0', true)}
            {@render applyCta('md', 'shrink-0')}
          </div>
        </div>
      </div>
    </div>

    {#if job.closed_at}
      {@const closed = formatDate(job.closed_at)}
      <div class="rounded-md border border-border bg-secondary px-4 py-3 text-sm">
        This position is no longer accepting applications{#if closed}
          (closed {closed}){/if}.
      </div>
    {/if}

    <!-- Tabs left, actions right, one rule under both — and no gap between the halves,
         because each draws its own stretch of that rule and a gap here is a gap in the
         line (the strip's own left padding separates them instead). The row is rendered
         whether or not there are tabs to put in it: a posting from an unknown employer
         has only the description, and the rule then simply closes the actions off. -->
    <div class="flex items-end justify-between">
      {#if contentTabs.length > 1}
        <TabStrip
          tabs={contentTabs}
          active={contentTab}
          onSelect={(id) => (contentTab = id)}
          label="Job details"
          {panelId}
          class="min-w-0 flex-1"
        />
      {:else}
        <div class="min-w-0 flex-1 border-b border-border" aria-hidden="true"></div>
      {/if}
      {@render actionStrip('hidden border-b border-border pb-2 lg:flex')}
    </div>

    <!-- The three states of one exchange, below the action strip because that strip is
         what raised the question: Apply was clicked there, so the answer belongs under
         the hand that asked rather than back up beside the title. They share one slot so
         that answering "Yes, save" replaces the question in place — split across the
         page, the confirmation would read as a second, unrelated banner appearing
         somewhere the reader was not looking. -->
    {#if showApplyPrompt && !applied}
      <div
        class="flex flex-wrap items-center justify-between gap-3 rounded-md border border-border bg-secondary px-4 py-3"
      >
        <span class="text-sm">Did you apply to this job?</span>
        <div class="flex items-center gap-2">
          <Button variant="primary" size="sm" onclick={confirmApplied}>Yes, save</Button>
          <Button variant="ghost" size="sm" onclick={dismissApplyPrompt}>No</Button>
        </div>
      </div>
    {/if}

    {#if justApplied}
      <div
        class="flex flex-wrap items-center justify-between gap-3 rounded-md border border-brand/30 bg-brand-muted px-4 py-3"
      >
        <span class="inline-flex items-center gap-1.5 text-sm font-medium text-brand-strong">
          <CheckCircle2 class="size-4 shrink-0" aria-hidden="true" /> Added to your board
        </span>
        <a
          href={resolve('/my/tracking')}
          class="text-sm font-medium text-brand-strong underline underline-offset-4"
        >
          View on your board →
        </a>
      </div>
    {/if}

    {#if autoApplyState.kind === 'queued'}
      <div
        class="flex flex-wrap items-center justify-between gap-3 rounded-md border border-brand/30 bg-brand-muted px-4 py-3"
      >
        <span class="inline-flex items-center gap-1.5 text-sm font-medium text-brand-strong">
          <CheckCircle2 class="size-4 shrink-0" aria-hidden="true" /> We're preparing a tailored CV — you'll get
          a notification to review it.
        </span>
      </div>
    {/if}
    {#if autoApplyError}
      <p class="text-sm text-destructive">{autoApplyError}</p>
    {/if}

    {#if showSignInPrompt}
      <div
        class="flex flex-wrap items-center justify-between gap-3 rounded-md border border-border bg-secondary px-4 py-3"
      >
        <span class="text-sm">Sign in to keep track of the jobs you apply to.</span>
        <div class="flex items-center gap-2">
          <Button variant="primary" size="sm" onclick={signUpFromGate}>Sign up</Button>
          <Button variant="ghost" size="sm" onclick={viewWithoutSignIn}>View without signing in</Button>
        </div>
      </div>
    {/if}

    {#if contentTabs.length > 1}
      <!-- One panel for every tab, its contents toggled by class rather than {#if}.
           Unmounting the inactive one would throw away the company the visitor already
           waited for, and re-render the description on every switch back. It also keeps
           every panel in the server-rendered HTML, so a crawler reads what a visitor
           would have to click for. -->
      <div id={panelId} role="tabpanel" aria-labelledby={tabStripId(panelId, contentTab)}>
        <div class={contentTab === 'description' ? 'flex flex-col gap-6' : 'hidden'}>
          {@render descriptionContent()}
        </div>
        {#if companySlug}
          <div class={contentTab === 'company' ? 'block' : 'hidden'}>
            {#key companySlug}
              <JobCompanyPanel
                slug={companySlug}
                name={job.company || 'this company'}
                active={contentTab === 'company'}
              />
            {/key}
          </div>
        {/if}
        <div class={contentTab === 'application' ? 'block' : 'hidden'}>
          <JobApplyForm form={applyForm} />
        </div>
      </div>
    {:else}
      {@render descriptionContent()}
    {/if}
  </div>

  <!-- Mobile apply CTA. It sticks to the bottom of the viewport while the job
       scrolls, then releases at the article's end so it never covers the related
       jobs below; the safe-area padding clears the home indicator. The bar is a
       frosted-glass panel (semi-transparent bg + backdrop-blur), full-bleed via
       negative margins that cancel the page gutter. pointer-events-none lets the
       description scroll under the glass, with pointer-events-auto re-enabling the
       button. Desktop uses the inline header button instead (hidden at lg). -->
  <div
    class="pointer-events-none sticky bottom-0 z-30 -mx-5 border-t border-border/40 bg-background/15 px-5 pb-[max(0.75rem,env(safe-area-inset-bottom))] pt-3 backdrop-blur-lg sm:-mx-4 sm:px-4 lg:hidden"
  >
    {@render applyCta('lg', 'pointer-events-auto w-full rounded-xl font-semibold shadow-lg')}
  </div>
</article>

{#if showReport}
  <ReportDialog slug={job.public_slug} onClose={() => (showReport = false)} />
{/if}
