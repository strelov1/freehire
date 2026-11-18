<script lang="ts">
  import { resolve } from '$app/paths';
  import { Bookmark, FileText, Lock } from '@lucide/svelte';
  import { companyLogoUrl } from '#lib/logo.js';
  import { cardLocation } from '#lib/cardLocation.js';
  import JobMatchBar from './JobMatchBar.svelte';
  import SourceBadge from './SourceBadge.svelte';
  import { api } from '#lib/api.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { track } from '#lib/analytics.js';
  import { cardTags, cardTagsFromCard } from '#lib/enrichment.js';
  import { computeClientMatch, resolveMatchState } from '#lib/jobMatch.js';
  import { profileStore } from '#lib/profile.svelte.js';
  import { foreignContentLang } from '#lib/seo.js';
  import { EntityLogo } from '#lib/ui/index.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { ghostBadge, supersedesReality } from '#lib/ghost.js';
  import { realityBadge } from '#lib/reality.js';
  import CredentialBadge from './CredentialBadge.svelte';
  import BackerBadge from './BackerBadge.svelte';
  import { credentialBadges } from '#lib/credentials.js';
  import { directApplyTooltip } from './DirectApplyBadge.svelte';
  import { appliedCompanyTooltip } from './AppliedCompanyBadge.svelte';
  import JobRowApplyGate from './JobRowApplyGate.svelte';
  import { JobCardActions } from './jobCardActions.svelte';
  import JobRowCornerActions from './JobRowCornerActions.svelte';
  import JobRowBadges from './JobRowBadges.svelte';
  import JobRowGeo from './JobRowGeo.svelte';
  import JobRowRailFacts from './JobRowRailFacts.svelte';
  import JobRowSavedNote from './JobRowSavedNote.svelte';
  import JobRowSkills from './JobRowSkills.svelte';
  import JobRowMetaLine from './JobRowMetaLine.svelte';
  import {
    rowBlurb,
    rowEquity,
    rowGeoTags,
    rowMetaFacts,
    rowQuestions,
    rowRegionTag,
    rowSalary,
    rowViews,
  } from './jobRowFacts';
  import type { JobRowProps } from './JobRow.props';
  import { countryLabel, skillLabel } from '#lib/facets.js';
  import { backerBadges } from '#lib/backers.js';
  import { timeAgo } from '#lib/utils.js';
  import { cardFreshnessBadges } from '#lib/freshness.js';
  import { hasViewed } from '#lib/viewedJobs.svelte.js';
  import { ensureSavedLoaded, isSaved } from '#lib/savedJobs.svelte.js';
  import { hasAppliedToCompany } from '#lib/appliedCompanies.svelte.js';

  // Single source of truth for how a job appears in any list (jobs list and
  // company detail). The whole card is a link to the job detail. The props, and what
  // `dimViewed`, `newTab`, `compact`, `footer` and `onHide` change, are told in JobRow.props.ts.
  let {
    job,
    dimViewed = true,
    newTab = false,
    compact = false,
    hideCompany = false,
    flat = false,
    footer,
    onHide,
    unsave = api.unsaveJob,
    clientMatch,
  }: JobRowProps = $props();

  const isViewed = $derived(dimViewed && hasViewed(job.public_slug));
  // Whether the reader has applied to this employer before, through any of its postings.
  // Only the catalogue's full posting carries `company_slug`; the tracking listing's card
  // does not, and every row there is the reader's own tracked job anyway.
  const appliedToCompany = $derived('company_slug' in job && hasAppliedToCompany(job.company_slug));

  const tags = $derived('enrichment' in job ? cardTags(job) : cardTagsFromCard(job));
  // Only the credential subset of job.collections renders here, so the signal row's
  // guard has to test that subset — a job carrying only editorial tags would
  // otherwise open an empty flex row and leave a stray margin under the title.
  const credentials = $derived(credentialBadges(job.collections));
  // Only the flags the stack actually draws count as said: a country past the cap hides
  // behind "+N", so the caption must still name it.
  const MAX_FLAGS = 5;
  const place = $derived(cardLocation(job.location, job.countries?.slice(0, MAX_FLAGS)));
  // The card's descriptive lines (see jobRowFacts.ts for what each omits and why).
  const blurb = $derived(rowBlurb(job));
  const salary = $derived(rowSalary(job));
  const equity = $derived(rowEquity(job));
  const questions = $derived(rowQuestions(job));
  // Top-level `skills` is the served (deterministic-dictionary) facet; the raw
  // `enrichment.skills` is kept in the JSONB and NOT served, so it's always absent
  // here — read the dictionary field so the card's skill chips actually populate.
  const skills = $derived(job.skills ?? []);
  // The two read-time signals. Both are attached by the catalogue's projection and neither
  // has ever been computed for the tracking listing, so a card simply has none — the rows
  // that render cards showed no badge before this change either.
  const reality = $derived('reality' in job ? job.reality : undefined);
  const ghost = $derived('ghost' in job ? job.ghost : undefined);
  // How recently it was posted is a key signal, so it leads the header.
  const posted = $derived(timeAgo(job.posted_at, locale()));
  const views = $derived(rowViews(job));
  // The direct-apply shortcut in the action row below. Only `jobview.Job` carries the
  // outbound `url` (and the `source` the apply-intent event reports) — `jobview.Card`
  // (saved / lists / tracking board) does not, so those surfaces render no button
  // rather than guessing a destination. See freehire#2755: widening Card is a separate,
  // deliberately deferred change (it touches CardInput, both call sites, and the SPA
  // contracts), so this narrows to the shape that already carries the field.
  // An empty url is a posting with nowhere outside to send the candidate: a vacancy taking
  // applications on freehire, whose Apply opens its own job page, where the form is. Only
  // `jobview.Job` carries that flag, like `url`.
  // A closed posting takes no applications, so it offers no Apply at all.
  const applyJob = $derived('url' in job && job.url && !job.closed_at ? job : null);
  const directApply = $derived('direct_apply' in job && job.direct_apply === true && !job.closed_at);
  // The freshness badge, through the card's own gate: a projection that carries no
  // reality signal earns none, because on those surfaces the posting date alone would
  // vouch for a job the signal was written to distrust. Everything else — the
  // thresholds, the wording, the closed-posting rule — is the shared rule's, so the card
  // and the job's own page cannot tell different stories about one posting.
  const freshness = $derived(cardFreshnessBadges(job.posted_at, reality, job.closed_at));

  const MAX_SKILLS = 5;
  const regionTag = $derived(rowRegionTag(job));
  const geoTags = $derived(rowGeoTags(job, tags));
  // On the employer's own page the rail (logo, name, date) is not drawn, so the date leads
  // the meta line instead — the same place a reader finds it relative to the title elsewhere.
  // On touch, Apply and Hide never hide, so the corner cluster is wide; the date then
  // moves from the rail to the meta line so the company name keeps its room.
  const touchActions = $derived(!compact && Boolean(applyJob || directApply));
  const touchPosted = $derived(touchActions && !hideCompany ? posted : null);
  const metaFacts = $derived(
    rowMetaFacts({ job, hideCompany, posted, views, tags, regionTag, geoTags, equity, questions }),
  );
  // The meta line continues whatever plain text precedes it — a freshness label, or the
  // reality age ("Open 87d") when it renders as text rather than a warning pill — so it
  // opens with a dot exactly then; without one the two ran together as "Open 87d 3 weeks ago".
  const metaLeadDot = $derived(
    freshness.length > 0 || (!supersedesReality(ghost, locale()) && realityBadge(reality)?.tone === 'muted'),
  );
  const shownSkills = $derived(skills.slice(0, MAX_SKILLS));
  const extraSkills = $derived(skills.length - MAX_SKILLS);

  // Card-level profile match, computed entirely in the browser (no per-card request):
  // the exact overlap between this job's skills and the signed-in viewer's profile
  // skills. The job is server-rendered; only the profile hydrates client-side, loaded
  // once and deduped across every card by the store (SSR-safe no-op for guests).
  $effect(() => {
    if (isAuthenticated()) profileStore.ensureLoaded();
  });
  const profileSkills = $derived(profileStore.profile?.skills ?? []);
  const matchState = $derived(
    resolveMatchState({
      jobSkills: skills,
      authenticated: isAuthenticated(),
      profileLoaded: profileStore.loaded,
      profileSkills,
    }),
  );
  // The viewer's skills as a lowercase set, only when there's a real match to show —
  // used to tint each skill chip (a skill you have vs one you're missing). Null in the
  // locked states, where the teaser below decides the tint instead.
  const haveSet = $derived(matchState === 'ready' ? new Set(profileSkills.map((s) => s.toLowerCase())) : null);
  const match = $derived(matchState === 'ready' ? (clientMatch ?? computeClientMatch(skills, profileSkills)) : null);

  // A viewer with no match to show — signed out, or signed in with no skills. It used to
  // drive a blurred strip and tinted chips on every card; a feed of blurred grey read as a
  // page that failed to load, so the card now shows neutral chips and one quiet invitation
  // in its action row. The blurred figures stay on the job's own page.
  const teaser = $derived((matchState === 'guest' || matchState === 'no-profile') && skills.length > 0);

  // One copy of the invitation, shared by the compact card's line and the normal card's
  // action-row button — so the two can't drift into different wording.
  const matchInvite = $derived(
    matchState === 'guest' ? 'Sign in to see your match' : 'Upload your CV to see your match',
  );

  // Whether the signed-in user has saved this job, read from the shared saved set
  // (loaded once and shared across cards). The bookmark reflects this and updates the
  // set on toggle, so every card for the same job stays in sync.
  const saved = $derived(isSaved(job.public_slug));
  $effect(() => {
    if (isAuthenticated()) void ensureSavedLoaded();
  });
  let showApplyGate = $state(false);

  // The save and hide buttons (see JobCardActions).
  const actions = new JobCardActions(
    () => job.public_slug,
    () => unsave,
    () => onHide,
  );

  // Only the title is inside the link. Its name starts with the visible title;
  // company disambiguates repeated roles, and the description carries key metadata.
  const linkName = $derived(`${job.title} at ${job.company || 'Unknown company'}`);
  const uid = $props.id();
  const summaryId = `${uid}-summary`;
  const summary = $derived(
    [
      isViewed ? 'Viewed' : '',
      appliedToCompany ? 'Applied to this company' : '',
      job.location ?? '',
      posted ? `Posted ${posted}` : '',
      job.application_deadline ? `Apply by ${job.application_deadline}` : '',
      salary ?? '',
      equity ?? '',
      questions ?? '',
    ]
      .filter(Boolean)
      .join(', '),
  );

  // The stretched link receives pointer hits instead of the metadata underneath.
  // Keep full labels and badge evidence available in its native card-wide tooltip.
  const linkTooltip = $derived(
    [
      linkName,
      summary,
      directApply ? directApplyTooltip : '',
      appliedToCompany ? appliedCompanyTooltip : '',
      shownSkills.length ? `Skills: ${shownSkills.map(skillLabel).join(', ')}` : '',
      ...(job.countries ?? []).slice(0, MAX_FLAGS).map(countryLabel),
      ...freshness.map((badge) => badge.tooltip),
      supersedesReality(ghost, locale()) ? ghostBadge(ghost, locale())?.tooltip : realityBadge(reality)?.tooltip,
      ...(!hideCompany ? credentials.map((badge) => badge.tooltip) : []),
      ...(!hideCompany ? backerBadges(job.collections).map((badge) => badge.alt) : []),
      job.ai_interview_reports ? `AI interview reports: ${job.ai_interview_reports}` : '',
    ]
      .filter(Boolean)
      .join('\n'),
  );

  // Apply-intent — fired regardless of auth, the same event and shape the job page's own
  // Apply button fires (JobView.svelte), so the funnel stays comparable across surfaces.
  // A signed-out click gets an explicit choice: sign in/up to track the application,
  // or continue to the employer without an account.
  function onApplyClick(e: MouseEvent) {
    if (applyJob) track('job_apply', { slug: applyJob.public_slug, source: applyJob.source });
    if (!isAuthenticated()) {
      e.preventDefault();
      showApplyGate = true;
    }
  }
</script>

<!-- The wrapper owns the card chrome and hover group. Navigation lives on the title;
     actions stay outside the link and above its stretched hit area. -->
<div
  data-testid="job-card"
  class={[
    'group relative transition hover:bg-accent',
    !flat && 'rounded-xl border border-border/70 bg-card hover:border-brand',
  ]}
>
  <!-- The title link stretches over this content area. Keeping the containing block
       here leaves footer actions outside it; corner controls sit above it at z-10. -->
  <div class={['relative isolate hover:opacity-100', compact ? 'p-3' : 'px-4 py-3.5']} class:opacity-90={isViewed}>
    <!-- Company + timestamp rail: a quiet eyebrow that yields the stage to the title.
       The name truncates to a single line, so a long company (e.g. "Veterinary
       Emergency Group (VEG)") keeps the logo centred and the card rhythm even
       instead of wrapping into a ragged multi-line header. -->
    <!-- Reserves the top-right corner for the Save overlay, so the
       timestamp never slides under it. Only a compact card keeps Save up here; a normal
       card has moved it into the action row at its foot, leaving the corner free. -->
    <!-- Not drawn on the employer's own page (`hideCompany`): with no company the rail would
       hold only small facts floating above the title, so they lead the meta line instead. -->
    <div
      class={[
        'flex items-center gap-2',
        !compact && onHide ? 'pr-20' : 'pr-11',
        touchActions && 'pointer-coarse:pr-30',
        hideCompany && 'hidden',
      ]}
    >
      <div class="flex min-w-0 items-center gap-2">
        {#if !hideCompany}
          <EntityLogo
            name={job.company || 'Unknown company'}
            src={companyLogoUrl(job.company) ?? undefined}
            shape="square"
            size="sm"
          />
          <span class="truncate text-sm font-medium text-muted-foreground">
            {job.company || 'Unknown company'}
          </span>
          <!-- Who backed the employer, next to the employer. A fact about the company,
           so it reads with the company name rather than joining the signal row
           below, which carries role-level facts and already wraps on a phone. The
           title link handles navigation, so the mark is display-only here. -->
          <BackerBadge collections={job.collections} class="shrink-0" />
        {/if}
      </div>
      <!-- The date reads with the company ("Stripe · 2 hours ago"), not pinned to the far
         edge where it floated alone beside the corner controls. -->
      {#if !hideCompany && (posted || views > 0 || isViewed || job.application_deadline)}
        <span class={['text-muted-foreground', touchActions && 'pointer-coarse:hidden']} aria-hidden="true">·</span>
      {/if}
      <JobRowRailFacts
        {isViewed}
        {views}
        {posted}
        showPosted={!!posted && !hideCompany}
        deadline={job.application_deadline}
        {touchActions}
      />
    </div>

    <!-- The title is the card's hero — a size up from the body with tight leading, so
       the eye lands on the role first.

       `lang` sits on the heading because adjacent postings may use different languages. -->
    <h3
      lang={foreignContentLang(job)}
      class={[
        'font-semibold leading-snug tracking-tight',
        compact ? 'mt-2 text-base' : 'text-base sm:text-lg',
        !compact && !hideCompany && 'mt-1.5',
        // With the rail hidden the title is the top line, so it reserves the corner controls' room the rail would have.
        hideCompany && (!compact && onHide ? 'pr-20' : 'pr-11'),
        hideCompany && touchActions && 'pointer-coarse:pr-30',
      ]}
    >
      <a
        href={resolve('/jobs/[slug]', { slug: job.public_slug })}
        target={newTab ? '_blank' : undefined}
        rel={newTab ? 'noopener' : undefined}
        aria-label={linkName}
        aria-describedby={summary ? summaryId : undefined}
        title={linkTooltip}
        class={[
          'block after:absolute after:inset-0 after:z-10 focus-visible:outline-none focus-visible:after:outline-2 focus-visible:after:-outline-offset-2 focus-visible:after:outline-ring',
          !flat && 'after:rounded-xl',
        ]}
      >
        <!-- Clamp only the text. The inset focus outline also stays visible when a feed
             clips its rows to a rounded container. -->
        <span class={compact ? 'line-clamp-1' : 'line-clamp-2'}>{job.title}</span>
      </a>
    </h3>

    <!-- Signal row: reality chip + the region/employment facets, grouped under the
       title as quiet outline chips so they read as metadata, not decoration. -->
    <!-- ai_interview_reports belongs in this guard on its own: a card can carry the
       label while having no reality verdict, no freshness, no tags, no countries and no
       credentials, and the row would then be suppressed with the badge inside it. -->
    {#if reality || freshness.length > 0 || metaFacts.length > 0 || credentials.length > 0 || job.ai_interview_reports || directApply || appliedToCompany || equity || questions || salary || (hideCompany && (posted || views > 0 || isViewed || job.application_deadline)) || touchPosted}
      <div class="mt-2 flex flex-wrap items-center gap-1.5">
        <JobRowBadges {job} {directApply} {appliedToCompany} {ghost} {reality} {freshness} />
        <!-- The role's facets, then the pay and the cost of applying, as one line of quiet
           text joined by dots rather than a row of outlined chips: the skills below are
           chips already, and two rows of identical outlines made every card noisy.
           Indexed keys: a tag's text could equal one of the trailing facts. -->
        <JobRowMetaLine {metaFacts} {salary} {touchPosted} {hideCompany} {isViewed} {metaLeadDot} />
        <!-- Register-backed employer credentials (visa-sponsor licences). A fact about
           the employer, so it sits with the metadata chips and carries its own
           disclaimer rather than reading as a promise about this role. -->
        <!-- Not on the employer's own page, whose header already states it for every row. -->
        {#if !hideCompany}<CredentialBadge collections={job.collections} plain />{/if}
      </div>
    {/if}

    <!-- The geography line: arrangement, then the flags, then the city the flags do not name. -->
    {#if geoTags.length || job.countries?.length || place}
      <JobRowGeo {geoTags} countries={job.countries} maxFlags={MAX_FLAGS} {place} location={job.location} />
    {/if}

    {#if blurb && !compact}
      <p class="mt-2 line-clamp-2 text-sm text-muted-foreground">{blurb}</p>
    {/if}

    <!-- The skills, as the card's one chip row (the pay rides the meta line above). -->
    {#if skills.length}
      <JobRowSkills {shownSkills} {extraSkills} {haveSet} />
    {/if}

    {#if 'source' in job && job.source}
      <div class="mt-2 flex min-w-0 flex-wrap items-center gap-1.5">
        <SourceBadge source={job.source} />
      </div>
    {/if}

    <!-- Card-level profile match: the real client-computed coverage bar, only for a
       signed-in viewer with a skills profile. -->
    <JobMatchBar {match} />
    {#if teaser && compact}
      <!-- The compact card has no action row, so its invitation stays a caption. -->
      <div class="mt-1.5 flex items-center gap-1.5 text-xs text-muted-foreground">
        {#if matchState === 'guest'}
          <Lock class="size-3.5 shrink-0" aria-hidden="true" />
        {:else}
          <FileText class="size-3.5 shrink-0" aria-hidden="true" />
        {/if}
        {matchInvite}
      </div>
    {/if}
  </div>

  <!-- The card link's description (see `summary`). `hidden` keeps it off the page and out
     of browse mode, where the same facts are read from the card itself; a description
     reference still reaches hidden content. -->
  {#if summary}
    <span id={summaryId} hidden>{summary}</span>
  {/if}

  {#if footer && compact}
    <!-- Optional in-card actions row (e.g. the assistant deck's rationale), divided from
       the content and rendered outside the <a> so its controls stay clickable. Its
       inline padding tracks the card's own, or the row would sit 4px out of line.
       A normal card has an action row of its own, and the caller's controls join it
       rather than stacking a second bordered row under the first. -->
    <div class="border-t border-border px-3 py-2.5">
      {@render footer()}
    </div>
  {/if}

  <!-- Save toggle. Outside the <a> (a sibling, not a descendant) either way, so activating
     it never navigates to the posting.

     Two presentations, one rule: a labelled button on a normal card, the older icon-only
     overlay in `compact`, where the narrow chat column has no width to spend on a word.
     The label is the point — saving now puts the job on the tracking board, and an
     unlabelled glyph in a corner asked people to guess that. It says which of the two
     states it is in rather than which action it performs, because the state is what a
     card in a long feed has to answer at a glance. -->
  {#if compact}
    <button
      type="button"
      onclick={actions.toggleSave}
      disabled={actions.saving}
      aria-pressed={saved}
      aria-label={saved ? 'Remove from your board' : 'Save to your board'}
      title={saved ? 'On your board' : 'Save to board'}
      class={[
        'absolute right-2.5 top-4 z-10 grid size-8 place-items-center rounded-lg transition hover:bg-accent hover:text-brand disabled:pointer-events-none disabled:opacity-50',
        saved ? 'text-brand' : 'text-muted-foreground',
      ]}
    >
      <Bookmark class="size-[1.05rem] {saved ? 'fill-current' : ''}" aria-hidden="true" />
    </button>
  {:else}
    <JobRowCornerActions
      slug={job.public_slug}
      title={job.title}
      {directApply}
      applyUrl={applyJob?.url ?? null}
      {onApplyClick}
      onHide={onHide ? actions.hide : undefined}
      hiding={actions.hiding}
      {saved}
      saving={actions.saving}
      onToggleSave={actions.toggleSave}
    />

    <!-- The caller's own controls, when it passes any, get the one row under the content. -->
    {#if footer}
      <div class="-mt-1 flex items-center gap-2 px-4 pb-3">
        {@render footer()}
      </div>
    {/if}
  {/if}

  {#if actions.justSaved && saved && !compact}
    <JobRowSavedNote slug={job.public_slug} />
  {/if}

  <JobRowApplyGate
    show={showApplyGate && !!applyJob}
    title={job.title}
    url={applyJob?.url ?? ''}
    onclose={() => (showApplyGate = false)}
  />
</div>
