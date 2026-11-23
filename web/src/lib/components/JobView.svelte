<script lang="ts">
  import { untrack } from 'svelte';
  import { ArrowRight, SquarePen } from '@lucide/svelte';
  import JobActionStrip from './JobActionStrip.svelte';
  import JobSaveButton from './JobSaveButton.svelte';
  import JobAsides from './JobAsides.svelte';
  import JobTitle from './JobTitle.svelte';
  import JobTabPanels from './JobTabPanels.svelte';
  import { api } from '#lib/api.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { autoApplyButtonState, jobCtaPlan, type JobCtaPlan } from '#lib/autoApplyButton.js';
  import { promptSignIn } from '#lib/signin.js';
  import { ensureAppliedCompaniesLoaded, hasAppliedToCompany } from '#lib/appliedCompanies.svelte.js';
  import { track } from '#lib/analytics.js';
  import { foreignContentLang } from '#lib/seo.js';
  import type { Display } from '#lib/generated/contracts.js';
  import type { Job } from '#lib/types.js';
  import { profileStore } from '#lib/profile.svelte.js';
  import { Button, TabStrip } from '#lib/ui/index.js';
  import JobFactsCard from './JobFactsCard.svelte';
  import JobPinnedHeader from './JobPinnedHeader.svelte';
  import JobApplyNotices from './JobApplyNotices.svelte';
  import JobSignInGate from './JobSignInGate.svelte';
  import { JobTailor } from '#lib/jobTailor.svelte.js';
  import { LazyDialog } from '#lib/lazyDialog.svelte.js';
  import { JobAutoApply } from './jobAutoApply.svelte';
  import { JobInteraction } from './jobInteraction.svelte';
  import { JobContentTabs } from './jobContentTabs.svelte';
  import JobPeopleLinks from './JobPeopleLinks.svelte';
  import JobPostingMeta from './JobPostingMeta.svelte';
  import JobScrollPadding from './JobScrollPadding.svelte';
  import JobProvenanceLine from './JobProvenanceLine.svelte';
  import JobClosedNotice from './JobClosedNotice.svelte';
  import ReportDialog from './ReportDialog.svelte';

  // The job is server-rendered: it arrives as a prop from the route's `load`, so
  // the article's content is in the initial HTML. Only the per-user interactions
  // below hydrate client-side.
  // `applyForm` is the employer's own screening form when its ATS publishes one —
  // null for most postings, which is the ordinary case and simply means one fewer tab.
  // `hasSimilar` says the route renders similar postings under this one (JobRelated, at
  // `#similar-jobs`), which is where a closed posting points its reader instead of Apply.
  let {
    job,
    applyForm = null,
    hasSimilar = false,
  }: { job: Job; applyForm?: Display | null; hasSimilar?: boolean } = $props();

  // A closed posting takes no applications: the closure notice moves up beside the title,
  // and every control that would start one — Apply, auto-apply, Tailor my CV, the phone's
  // sticky bar that carries them — is withheld rather than left to lead somewhere dead.
  // Save stays: keeping a closed posting for reference is still a reasonable thing to do.
  const closed = $derived(job.closed_at != null);

  // Set on the two subtrees below that carry the posting's own words; undefined
  // (so unset) when it is English. See foreignContentLang for why the document
  // stays `en` regardless.
  const contentLang = $derived(foreignContentLang(job));

  // The signed-in user's interaction with this job (see JobInteraction).
  const tracking = new JobInteraction(() => job);
  // Signed-out gate: the "Apply" click offers sign-in before opening the posting.
  let showSignInPrompt = $state(false);
  let mobileSignInFeedback: HTMLDivElement | undefined = $state();
  // The report dialog (a problem-with-this-job complaint) opens over the page.
  let showReport = $state(false);
  // The application form of a vacancy its employer publishes here (job.direct_apply). The
  // dialog is imported on the click that opens it, like ReferralBlock's modal: only those
  // few vacancies ever show it, so it stays out of every other job page's module graph.
  const directApply = new LazyDialog(() => import('./DirectApplyDialog.svelte'));
  const referralMessage = new LazyDialog(() => import('./ReferralMessageDialog.svelte'));

  /** Load the drafting dialog only when a signed-in seeker opens it. */
  function openReferralMessage() {
    if (!isAuthenticated()) {
      promptSignIn();
      return;
    }
    void referralMessage.show();
  }
  // Whether THIS candidate's application reached the employer — read from the application
  // itself, never from tracking: a vacancy marked applied by hand must still take the
  // application, and a delivered one must not offer Apply again if tracking lagged.
  let directApplied = $state(false);
  const applied = $derived(tracking.current?.applied_at != null);
  // The reader applied to this EMPLOYER before, through any of its postings. The title row
  // shows it only when this posting is not itself applied to: "Applied" already says more,
  // and both chips side by side would state one fact twice.
  const appliedToCompany = $derived(hasAppliedToCompany(job.company_slug));
  const saved = $derived(tracking.current?.saved_at != null);

  // The content tabs: Description, Company, Application, Discussion (see JobContentTabs).
  const tabs = new JobContentTabs(
    () => job,
    () => closed,
    () => applyForm,
  );
  // Per-instance so a second JobView on one page can't claim the same panel, which
  // would leave both strips' aria-controls pointing at the first one's panel.
  const panelId = $props.id();

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
    tracking.reset();
    showSignInPrompt = false;
    directApply.close();
    referralMessage.close();
    directApplied = false;
    if (!isAuthenticated()) return; // effects run client-only, so no browser guard needed
    untrack(ensureAppliedCompaniesLoaded);
    if (job.direct_apply) {
      api
        .getDirectApplyForm(slug)
        .then((f) => {
          if (job.public_slug === slug) directApplied = f.applied;
        })
        .catch(() => {});
    }
    // The Avoid control below reads the profile's excluded_sources; without this its mark
    // would be wrong on a first load until something else happened to fetch the profile.
    untrack(() => profileStore.ensureLoaded());
    tracking.record(slug);
  });

  function trackApplyClick() {
    track('job_apply', { slug: job.public_slug, source: job.source });
    if (job.source === 'employer' && !closed) {
      void api.recordEmployerVacancyApplyClick(job.public_slug).catch(() => {});
    }
  }

  // The Apply link opens the external posting; once the user has gone to apply,
  // offer the "Did you apply?" choice (only when signed in and not already applied).
  // Signed-out visitors are gated first: the click is intercepted (the link does
  // not open) and a sign-in offer is shown instead — the posting opens only via
  // "View without signing in" below.
  function onApplyClick(event: MouseEvent) {
    // Apply-intent — fired regardless of auth (the CTA click is the funnel step).
    trackApplyClick();
    if (!isAuthenticated()) {
      event.preventDefault();
      showSignInPrompt = true;
      return;
    }
    if (!applied) tracking.askApplied = true;
  }

  // A vacancy taking applications here has no outside page: Apply opens the form instead
  // of a link, and the "did you apply?" prompt has nothing to ask — sending the form IS
  // the application, and the server records it in the tracker.
  async function onDirectApplyClick() {
    trackApplyClick();
    if (!isAuthenticated()) {
      showSignInPrompt = true;
      return;
    }
    // Offline or a deploy swap leaves it closed and the button live, so a retry works.
    await directApply.show();
  }

  // Saving requires an account.
  function onSaveClick() {
    if (!isAuthenticated()) {
      promptSignIn();
      return;
    }
    void tracking.toggleSave();
  }

  // Reporting also requires an account (the report is attributed to the user).
  function onReportClick() {
    if (!isAuthenticated()) {
      promptSignIn();
      return;
    }
    showReport = true;
  }

  // Auto-apply (openspec/changes/auto-apply-submit-trigger): Pro (or above) and one of the
  // ATS providers auto-apply can queue against — both are gated on VISIBILITY (the plan JobAutoApply reads),
  // not just what happens on click.
  const autoApplyQueue = new JobAutoApply(() => job);
  const autoApplyState = $derived(
    autoApplyButtonState(
      job.source,
      autoApplyQueue.overrideStatus ?? job.auto_apply_status,
      applied,
      autoApplyQueue.isPro,
      closed,
    ),
  );
  // How loud each of the two CTAs is, and what they say. The table and the rule it keeps
  // ("never two primaries; one wherever an action remains") live in autoApplyButton.ts,
  // where they unit-test without mounting this component.
  const cta = $derived(jobCtaPlan(autoApplyState));
  const strip = $derived({
    saved,
    onSaveClick,
    onReportClick,
    applyUrl: cta.autoApply?.leads ? job.url : null,
    applyLabel: cta.external.label,
    onApplyClick,
  });

  // Tailoring: the cached fit analysis, whether there is a CV, and the start (see JobTailor).
  const tailor = new JobTailor(() => job);

  function onAutoApplyClick() {
    if (!isAuthenticated()) {
      promptSignIn();
      return;
    }
    void autoApplyQueue.start();
  }
</script>

<!-- The link out to the posting's own site, as a BUTTON. Rendered three times: beside the
     title on desktop, in the pinned header once that title scrolls away, and in the mobile
     sticky bar at the end of the article — all three passing `cta.external`, so the same
     link never reads at two ranks on one page. Size and layout classes are the only
     difference, which is why they share this snippet.
     A fourth copy of the link exists and does NOT come through here: the phone-only anchor
     in `JobActionStrip`, which is quiet strip furniture rather than a button. It repeats the
     `rel`/`target`/handler below on purpose — see the note there.
     `external` decides only the WORD. The treatment does not vary: the brand fill belongs
     to `Tailor my CV` in every state, so this link is an outline button whether it reads
     `Apply` or `Apply on company site`, and the demotion is carried by the word and by its place in
     the row. The destination, the target and the click handler are the same button either
     way, which is what keeps the apply-intent event comparable across postings whether or
     not auto-apply offered to do it instead.
     nofollow: the destination is the posting's own site, which the catalogue never
     vetted — the same stance the description sanitizer takes on in-body links
     (internal/sources/sanitize.go). Without it a submitted vacancy buys a followed
     link from every job page, which is what the SEO submissions are actually after. -->
{#snippet applyCta(size: 'md' | 'lg', className: string, external: JobCtaPlan['external'])}
  {#if job.direct_apply}
    <Button
      variant="outline"
      {size}
      onclick={onDirectApplyClick}
      disabled={directApplied || directApply.loading}
      class={className}
    >
      {directApplied ? 'Application sent' : external.label}
      {#if !directApplied}<ArrowRight class="size-4" />{/if}
    </Button>
  {:else}
    <Button
      variant="outline"
      {size}
      href={job.url}
      target="_blank"
      rel="nofollow noopener noreferrer"
      onclick={onApplyClick}
      class={className}
    >
      {external.label}
      <ArrowRight class="size-4" />
    </Button>
  {/if}
{/snippet}

{#snippet signInFeedback()}
  {#if showSignInPrompt}
    <JobSignInGate directApply={!!job.direct_apply} url={job.url} onclose={() => (showSignInPrompt = false)} />
  {/if}
{/snippet}

<!-- The page's one primary call to action, and the only button on it carrying the brand
     fill. It is here rather than in the Profile-match sidebar, where it used to sit under a
     score, a bar and up to thirty skill chips: applying sends the reader to somebody else's
     site, tailoring is what this page is for, and the CTA row should say so.
     Rendered in all three CTA positions — the title row, the pinned header, the phone's
     sticky bar — because a primary CTA that exists at one width and not another is the same
     defect as one that does not exist at all.
     Nothing states the allowance beside it. ConfirmTailorDialog, which the click raises,
     already says what a session costs and what today has left, at the moment of the commit;
     a count in two places is a count that can disagree. -->
{#snippet tailorCta(size: 'md' | 'lg', className: string)}
  {#if tailor.canTailor}
    <Button variant="cta" {size} disabled={tailor.busy} onclick={tailor.start} class={className}>
      Tailor my CV
      <SquarePen class="size-4" aria-hidden="true" />
    </Button>
  {/if}
{/snippet}

<!-- Auto-apply (openspec/changes/auto-apply-submit-trigger): beside the apply link, not a
     replacement for it — auto-apply still goes through the same ATS in the end, this
     button only starts the tailor-then-review sequence. What it SAYS comes from the CTA
     plan; how loud it is does not vary, the brand fill belonging to `Tailor my CV`. The
     only thing decided here is that a submission already in flight also disables it, which
     is a fact about THIS component's request rather than about the posting.
     The `Pro` marker is a span, not the `Badge` primitive: Badge's variants carry their own
     background and foreground, none of which read as a plan marker. It takes the PAGE's
     `foreground` for its fill and `background` for its text — near-black on white in the
     light theme, near-white on black in the dark one — so it stays the highest contrast
     thing on the button in either. A tint of the button's own foreground was the first try
     and it dissolved into the fill. -->
{#snippet autoApplyCta(size: 'md' | 'lg', className: string)}
  {#if cta.autoApply}
    {@const autoApply = cta.autoApply}
    <Button
      variant="secondary"
      {size}
      disabled={autoApply.disabled || autoApplyQueue.submitting}
      onclick={onAutoApplyClick}
      class={className}
    >
      {autoApply.label}
      {#if autoApply.pro}
        <span class="rounded-sm bg-foreground px-1.5 py-0.5 text-xs font-semibold leading-none text-background">
          Pro
        </span>
      {/if}
    </Button>
  {/if}
{/snippet}

<!-- The call-to-action buttons: the page's answer to "what do I do with this posting".
     They ride the title's own row rather than the tab row below it, because the tab row's
     other half is the content TabStrip and the strip is the half that cannot shrink — every
     control added here used to come straight out of the tab labels, which had been squeezed
     to a scrolling sliver by the time there were six.
     Left to right in ascending rank — auto-apply, the way out to the posting, then
     `Tailor my CV` — so the brand fill lands at the row's end rather than in its middle.
     `hidden lg:flex`: below lg the sticky bottom bar carries them instead, minus whichever
     apply control the plan did not pick. -->
{#snippet ctaGroup()}
  {#if !closed}
    <div class="hidden shrink-0 items-center gap-2 lg:flex">
      {@render autoApplyCta('md', 'shrink-0')}
      {@render applyCta('md', 'shrink-0', cta.external)}
      {@render tailorCta('md', 'shrink-0')}
    </div>
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
  <JobProvenanceLine {job} {closed}>
    {#snippet meta()}<JobPostingMeta {job} className="ml-auto hidden shrink-0 lg:flex" />{/snippet}
  </JobProvenanceLine>

  <header class="flex flex-col gap-3 lg:col-start-2 lg:row-start-2">
    <JobTitle title={job.title} lang={contentLang} {applied} {appliedToCompany} />

    <JobClosedNotice {job} {hasSimilar} />

    <!-- The CTAs take a row of their own under the title rather than riding its right
         edge: the title is the longest string on the page and the buttons are the widest
         fixed thing beside it, so sharing a line meant a two-word title left them adrift in
         the middle of the page and a long one pushed them down anyway. Left-aligned under
         the title they read as its next step; right-aligned they floated with nothing to
         their left. -->
    <!-- From lg the quiet actions (Report, Save) share the CTAs' row, at its far
         end, rather than the tab row: there they read as more tabs, and every pixel they
         took came out of the tab labels. -->
    <div class="hidden items-center justify-between gap-4 lg:flex">
      {@render ctaGroup()}
      <JobActionStrip className="-mr-1 flex" {...strip} />
    </div>

    <!-- Below lg the provenance line already spends its width on the company name and its
         badges, so the dates read here instead — a caption under the title rather than
         right-aligned, which on one phone-width column would only look adrift. -->

    <!-- Keep the metadata left and the compact actions at the right edge, even when
         longer metadata wraps onto its own line. -->
    <div class="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 border-b border-border pb-2 lg:hidden">
      <JobPostingMeta {job} className="flex min-w-0 flex-wrap" />
      <JobActionStrip className="ml-auto -mr-2 flex max-w-full flex-wrap justify-end" iconOnly {...strip} />
    </div>

    <JobAsides {job} onTailor={() => void tailor.start()} />
  </header>

  <JobFactsCard {job} matchAnalysis={tailor.analysis} {closed} />

  <div class="relative flex min-w-0 flex-col gap-6 lg:col-start-2 lg:row-start-3">
    <JobPinnedHeader {job} lang={contentLang}>
      {#snippet actions()}
        <JobSaveButton {saved} onclick={onSaveClick} className="size-9 rounded-md px-0" iconOnly />
        {#if !closed}
          {@render autoApplyCta('md', 'shrink-0')}
          {@render applyCta('md', 'shrink-0', cta.external)}
          {@render tailorCta('md', 'shrink-0')}
        {/if}
      {/snippet}
    </JobPinnedHeader>

    <div class="flex items-end justify-between">
      <TabStrip
        tabs={tabs.tabs}
        active={tabs.current}
        onSelect={tabs.select}
        label="Job details"
        {panelId}
        class="min-w-0 flex-1"
      />
      {#if job.company?.trim()}
        <JobPeopleLinks
          slug={job.public_slug}
          company={job.company.trim()}
          {closed}
          loading={referralMessage.loading}
          ondraft={() => void openReferralMessage()}
        />
      {/if}
    </div>

    {#if job.company?.trim() && !closed && referralMessage.failed}
      <span role="alert" class="text-sm text-destructive">Could not open the draft form. Please try again.</span>
    {/if}

    <JobApplyNotices
      askApplied={tracking.askApplied && !applied}
      justApplied={tracking.justApplied}
      queued={autoApplyState.kind === 'queued'}
      slug={job.public_slug}
      error={autoApplyQueue.error}
      onconfirm={tracking.confirmApplied}
      ondismiss={tracking.dismiss}
    />

    {#if directApply.open && directApply.component}
      <directApply.component
        slug={job.public_slug}
        title={job.title}
        onClose={directApply.close}
        onSent={tracking.sentCallback(() => (directApplied = true))}
        onTailor={() => {
          directApply.close();
          void tailor.start();
        }}
      />
    {/if}

    <!-- `contents`, not `block`: an empty wrapper still took a slot in the column's gap and
         doubled the space between the tab row and the description. -->
    <div class="hidden lg:contents">{@render signInFeedback()}</div>

    <JobTabPanels {panelId} {tabs} {job} lang={contentLang} {applyForm} />
  </div>

  <!-- Mobile apply CTA. It sticks to the bottom of the viewport while the job
       scrolls, then releases at the article's end so it never covers the related
       jobs below; the safe-area padding clears the home indicator. The bar is a
       near-opaque panel with a top rule, full-bleed via negative margins that cancel
       the page gutter. Not frosted glass: a `backdrop-filter` on a sticky bar is
       re-rasterised on every scroll frame (the cost TopBar avoids for its own
       reasons), and over a 15% background the description read straight through
       the buttons. pointer-events-none lets a swipe that starts on the bar's gutter
       still scroll the page, with pointer-events-auto re-enabling the buttons.
       Desktop uses the inline header buttons instead (hidden at lg).
       Two controls, side by side at half width each rather than stacked, so the bar is no
       taller than it was with one: `Tailor my CV` — the page's primary CTA, which must not
       be a thing only a desktop reader can reach — and exactly ONE way to apply. Which one
       is the plan's whole job: auto-apply where that can be started, the apply link
       everywhere else. The control that loses does not vanish — the quiet strip under the
       title picks the apply link up whenever auto-apply has taken this bar. -->
  {#if !closed}
    <div
      class="pointer-events-none sticky bottom-0 z-30 -mx-5 flex flex-col gap-2 border-t border-border bg-background/95 px-5 pb-safe pt-3 shadow-lg sm:-mx-4 sm:px-4 lg:hidden"
    >
      <div class="pointer-events-auto lg:hidden" bind:this={mobileSignInFeedback}>
        {@render signInFeedback()}
      </div>
      <div class="pointer-events-auto flex gap-2">
        {@render tailorCta('lg', 'flex-1 rounded-xl font-semibold shadow-lg')}
        {#if cta.autoApply?.leads}
          {@render autoApplyCta('lg', 'flex-1 rounded-xl font-semibold shadow-lg')}
        {:else}
          {@render applyCta('lg', 'flex-1 rounded-xl font-semibold shadow-lg', cta.external)}
        {/if}
      </div>
    </div>
  {/if}
</article>

{#if showReport}
  <ReportDialog slug={job.public_slug} companySlug={job.company_slug} onClose={() => (showReport = false)} />
{/if}

{#if referralMessage.open && referralMessage.component}
  <referralMessage.component slug={job.public_slug} company={job.company} onClose={referralMessage.close} />
{/if}

<JobScrollPadding {closed} signInPrompt={showSignInPrompt} feedbackEl={mobileSignInFeedback} />
