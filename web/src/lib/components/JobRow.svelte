<script lang="ts">
  import { goto } from '$app/navigation';
  import type { Snippet } from 'svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { Bookmark, Check, ExternalLink, Eye, EyeOff, FileText, Lock, MapPin } from '@lucide/svelte';
  import { companyLogoUrl } from '#lib/logo.js';
  import CountryFlagStack from './CountryFlagStack.svelte';
  import { cardLocation } from '#lib/cardLocation.js';
  import JobMatchBar from './JobMatchBar.svelte';
  import { api } from '#lib/api.js';
  import { currentUser, isAuthenticated } from '#lib/auth.svelte.js';
  import { promptSignIn, signinUrl } from '#lib/signin.js';
  import { track } from '#lib/analytics.js';
  import { cardTags, cardTagsFromCard, formatSalary, regionLabel, workArrangement } from '#lib/enrichment.js';
  import { computeClientMatch, resolveMatchState } from '#lib/jobMatch.js';
  import { profileStore } from '#lib/profile.svelte.js';
  import { foreignContentLang, metaDescription } from '#lib/seo.js';
  import type { Job, JobCard } from '#lib/types.js';
  import { Badge, Button, EntityLogo } from '#lib/ui/index.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { supersedesReality } from '#lib/ghost.js';
  import { realityBadge } from '#lib/reality.js';
  import CredentialBadge from './CredentialBadge.svelte';
  import BackerBadge from './BackerBadge.svelte';
  import { credentialBadges } from '#lib/credentials.js';
  import AIInterviewBadge from './AIInterviewBadge.svelte';
  import DirectApplyBadge from './DirectApplyBadge.svelte';
  import AppliedCompanyBadge from './AppliedCompanyBadge.svelte';
  import GhostBadge from './GhostBadge.svelte';
  import RealityBadge from './RealityBadge.svelte';
  import SkillIcon from './SkillIcon.svelte';
  import { skillLabel } from '#lib/facets.js';
  import { formatCount, timeAgo } from '#lib/utils.js';
  import { applyQuestionsLabel, equityLabel } from '#lib/jobFacts.js';
  import { cardFreshnessBadges } from '#lib/freshness.js';
  import { hasViewed } from '#lib/viewedJobs.svelte.js';
  import { ensureSavedLoaded, isSaved, markSaved, markUnsaved } from '#lib/savedJobs.svelte.js';
  import { hasAppliedToCompany } from '#lib/appliedCompanies.svelte.js';
  import { markDismissed, markUndismissed } from '#lib/dismissedJobs.svelte.js';

  // Single source of truth for how a job appears in any list (jobs list and
  // company detail). The whole card is a link to the job detail.
  //
  // `dimViewed` dims the card when the signed-in user has already viewed this
  // job, so the browse list shows what's been seen. The My Jobs surfaces (where
  // every card is viewed by definition) pass `dimViewed={false}` to opt out.
  // `newTab` opens the job in a new browser tab (used when the card is rendered
  // inside the assistant chat, so the conversation stays open). Both default off so the
  // jobs list / company pages are unchanged.
  //
  // `compact` is the narrow-chat-column card, and it decides more than density. As well
  // as the smaller padding and title, the one-line title and the dropped blurb, it
  // selects the card's whole control inventory: there is no action row, so Save stays
  // the corner glyph it always was and `onHide` has nowhere to render. The two concerns
  // are fused on purpose — the column is ~360px wide, and every control that earns a
  // word on a full card has to give it back here. Splitting them would be a second flag
  // whose only honest value is the same as this one's.
  //
  // `footer` is an optional actions row rendered inside the card, below the link
  // content (a sibling of the <a>, never nested in it — so its interactive controls
  // don't fight the card's navigation). The hidden list passes an un-hide control here,
  // and gets it as a second row under the card's own Save row.
  //
  // `onHide` is the feed's hook for the "hide this job" gesture: when set on a NON-compact
  // card (only the browse feed passes it), the action row gains a hover-revealed hide
  // control, and after a successful dismiss it calls back with the slug so the feed can
  // surface an undo affordance. Surfaces that reuse JobRow without it (saved/hidden
  // lists, tracking board, assistant chat) get no hide control — leaving it out scopes
  // the gesture to the feed.
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
  }: {
    // Either the catalogue's full posting or the tracking listing's card. The row draws the
    // same thing from both: the card carries a server-cut blurb where the posting carries the
    // whole description, and the two facet fields a full Job keeps inside `enrichment` arrive
    // flat. Everything the card omits is optional here already.
    job: Job | JobCard;
    dimViewed?: boolean;
    newTab?: boolean;
    compact?: boolean;
    // On the employer's own page every card would repeat its logo, name and backers.
    hideCompany?: boolean;
    // A row in a divided list (the feed draws the frame and the dividers), not a framed card.
    flat?: boolean;
    footer?: Snippet;
    onHide?: (slug: string) => void;
    /** Allows a saved list to serialize removal with pagination. */
    unsave?: (slug: string) => Promise<unknown>;
  } = $props();

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
  // A one-line blurb under the title so a card conveys what the job is without
  // opening it. Prefer the clean model-written summary, but only tech jobs are
  // enriched — fall back to a plain-text snippet of the raw (HTML) posting so
  // non-tech jobs still show a description. metaDescription strips the tags.
  const blurb = $derived(
    'enrichment' in job
      ? job.enrichment?.summary || metaDescription(job.description ?? '', 220)
      : metaDescription(job.blurb ?? '', 220),
  );
  // Salary lives in the enrichment, which a card does not carry: the tracking lists never
  // showed it (their rows came from the same listing) and a row without one simply omits it.
  const salary = $derived('enrichment' in job && job.enrichment ? formatSalary(job.enrichment) : null);
  // Two facts only the catalogue's full posting carries, each absent when unknown — and
  // absent renders nothing: "no equity" and "a long form" are claims we cannot make.
  const equity = $derived('equity' in job ? equityLabel(job.equity) : null);
  // A closed posting takes no applications, so what its form asks is beside the point.
  const questions = $derived(
    'apply_questions' in job && !job.closed_at ? applyQuestionsLabel(job.apply_questions) : null,
  );
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
  // How many people have opened the posting — the materialized counter, maintained
  // offline from the access logs (never counted on this read). A card projection does
  // not carry it, so the `in` narrowing is what lets those rows simply omit it, the
  // same way they omit salary. Zero renders nothing at all: "0 views" reads as a dead
  // figure rather than as information.
  const views = $derived('view_count' in job ? job.view_count : 0);
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
  // The region is dropped when the flags already name the country ("UK · 🇬🇧 United
  // Kingdom" said one place twice); with no country it is the only geography and stays.
  const regionTag = $derived(job.countries?.length ? regionLabel({ regions: job.regions ?? [] }) : null);
  // Where the role can be done from — the arrangement ("Remote") and, when no flag names a
  // country, the region — reads as one thought with the flags and the city, so the three
  // take a line of their own under the facts instead of being scattered across the end of it.
  const geoTags = $derived(
    [
      workArrangement({ work_mode: job.work_mode ?? '' }),
      job.countries?.length ? null : regionLabel({ regions: job.regions ?? [] }),
    ].filter((t): t is string => !!t && tags.includes(t)),
  );
  // On the employer's own page the rail (logo, name, date) is not drawn, so the date leads
  // the meta line instead — the same place a reader finds it relative to the title elsewhere.
  // On touch, Apply and Hide never hide, so the corner cluster is wide; the date then
  // moves from the rail to the meta line so the company name keeps its room.
  const touchActions = $derived(!compact && Boolean(applyJob || directApply));
  const touchPosted = $derived(touchActions && !hideCompany ? posted : null);
  const metaFacts = $derived(
    [
      // With no rail on the employer's own page, its date, views and deadline lead this line.
      ...(hideCompany
        ? [
            posted,
            views > 0 ? `${formatCount(views)} ${views === 1 ? 'view' : 'views'}` : null,
            job.application_deadline ? `Apply by ${job.application_deadline}` : null,
          ]
        : []),
      ...tags.filter((t) => t !== regionTag && !geoTags.includes(t)),
      equity,
      questions,
    ].filter((f): f is string => Boolean(f)),
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
  const match = $derived(matchState === 'ready' ? computeClientMatch(skills, profileSkills) : null);

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

  // A chip is red when the viewer's own profile lacks the skill — the same source as the
  // strip below, so the chips and the percentage can never tell different stories. With
  // no profile to compare against there is nothing to tint: the chips are plain facts.
  function chipVariant(skill: string): 'brand' | 'missing' | 'outline' {
    if (haveSet) return haveSet.has(skill.toLowerCase()) ? 'brand' : 'missing';
    return 'outline';
  }

  // Whether the signed-in user has saved this job, read from the shared saved set
  // (loaded once and shared across cards). The bookmark reflects this and updates the
  // set on toggle, so every card for the same job stays in sync.
  const saved = $derived(isSaved(job.public_slug));
  $effect(() => {
    if (isAuthenticated()) void ensureSavedLoaded();
  });
  // Guards against a double-click racing two requests for the same job.
  let saving = $state(false);
  // Set by a save made from THIS card, so the "view it on your board" strip answers
  // the click that just happened rather than decorating every saved card in a feed.
  let justSaved = $state(false);
  let showApplyGate = $state(false);

  // Toggle the save mark. Optimistic: flip the shared set first so the bookmark
  // fills instantly, then confirm with the server and roll back on failure. A
  // signed-out click routes to sign-in instead (no auto-save afterwards). The
  // button is an overlay sibling of the card link — not a descendant — so this
  // never triggers the card's navigation.
  async function toggleSave() {
    if (!isAuthenticated()) {
      promptSignIn();
      return;
    }
    if (saving) return;
    saving = true;
    const userID = currentUser()?.id;
    let wasSaved = saved;
    let changed = false;
    try {
      // Read the final set before changing it: a late initial load must never undo
      // the user's toggle, and a handoff to another account cancels this action.
      await ensureSavedLoaded();
      if (currentUser()?.id !== userID) return;
      wasSaved = isSaved(job.public_slug);
      justSaved = false;
      if (wasSaved) markUnsaved(job.public_slug);
      else markSaved(job.public_slug);
      changed = true;
      if (wasSaved) {
        await unsave(job.public_slug);
      } else {
        await api.saveJob(job.public_slug);
        justSaved = true;
      }
    } catch {
      if (changed && currentUser()?.id === userID) {
        if (wasSaved) markSaved(job.public_slug);
        else markUnsaved(job.public_slug);
      }
    } finally {
      saving = false;
    }
  }

  // Guards against a double-click firing two dismiss requests for the same job.
  let hiding = $state(false);

  // Hide (dismiss) the job from the feed. Optimistic: mark it hidden in the shared
  // set first so the feed drops the card instantly, then confirm with the server and
  // roll back on failure. A signed-out click routes to sign-in instead. On success we
  // hand the slug to the feed (onHide) so it can surface an undo affordance. Like the
  // save button, this is an overlay sibling of the card link, so it never navigates.
  async function hide() {
    if (!isAuthenticated()) {
      promptSignIn();
      return;
    }
    if (hiding) return;
    hiding = true;
    markDismissed(job.public_slug);
    try {
      await api.dismissJob(job.public_slug);
      onHide?.(job.public_slug);
    } catch {
      markUndismissed(job.public_slug);
    } finally {
      hiding = false;
    }
  }

  // The card link's accessible name. The whole card is one <a>, so left to itself its name
  // is every word on the card — the blurb, each chip, each skill — hundreds of characters
  // read out before a screen-reader user learns which job it is. The name is "Title at
  // Company" instead, and the few facts worth hearing before opening the posting go in its
  // description — the full location line among them, since one employer's postings often
  // share a title and differ only by city. Browse mode still reads the card's full visible text.
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

  function signUpFromApplyGate() {
    showApplyGate = false;
    // Register, then come back to this list; the layout routes a new account through
    // /onboarding once, and that page can skip straight back here.
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- signinUrl() wraps resolve('/signin'); the rule can't see through the appended query
    void goto(signinUrl({ returnTo: page.url.pathname + page.url.search }));
  }

  function signInFromApplyGate() {
    showApplyGate = false;
    promptSignIn();
  }

  function viewApplyWithoutSigningIn() {
    showApplyGate = false;
    if (applyJob) window.open(applyJob.url, '_blank', 'noopener,noreferrer');
  }
</script>

<!-- The card chrome (border, background, hover) lives on this wrapper, not the <a>,
     so an optional footer row can sit inside the same bordered box as a sibling of
     the link — interactive footer controls never nest inside the navigation <a>.
     `group` lets the hover-revealed hide control fade in on card hover. -->
<div
  data-testid="job-card"
  class={[
    'group relative transition hover:bg-accent',
    !flat && 'rounded-xl border border-border/70 bg-card hover:border-brand',
  ]}
>
  <a
    href={resolve('/jobs/[slug]', { slug: job.public_slug })}
    target={newTab ? '_blank' : undefined}
    rel={newTab ? 'noopener' : undefined}
    aria-label={linkName}
    aria-describedby={summary ? summaryId : undefined}
    class={['block hover:opacity-100', compact ? 'p-3' : 'px-4 py-3.5']}
    class:opacity-90={isViewed}
  >
    <!-- Company + timestamp rail: a quiet eyebrow that yields the stage to the title.
       The name truncates to a single line, so a long company (e.g. "Veterinary
       Emergency Group (VEG)") keeps the logo centred and the card rhythm even
       instead of wrapping into a ragged multi-line header. -->
    <!-- Reserves the top-right corner for the Save overlay outside this link, so the
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
           whole card is a link, so the mark is display-only here. -->
          <BackerBadge collections={job.collections} class="shrink-0" />
        {/if}
      </div>
      <!-- The date reads with the company ("Stripe · 2 hours ago"), not pinned to the far
         edge where it floated alone beside the corner controls. -->
      {#if !hideCompany && (posted || views > 0 || isViewed || job.application_deadline)}
        <span class={['text-muted-foreground', touchActions && 'pointer-coarse:hidden']} aria-hidden="true">·</span>
      {/if}
      <div class="flex shrink-0 items-center gap-1.5 text-muted-foreground">
        {#if isViewed}
          <!-- A quiet "you've seen this" marker, paired with the lighter dim on the card
             so viewed jobs recede without becoming hard to read. The dim is 90%, not
             80%: on the hover fill (`accent`) 80% took muted text to 4.15:1 in the light
             theme, under the 4.5:1 floor; 90% keeps it at 5.2:1 in both themes.
             A check, not an eye: the eye beside it carries the PUBLIC view count, and
             two eyes in one rail meaning different things cannot be told apart. -->
          <Check class="size-3.5" aria-label="You have viewed this" />
        {/if}
        {#if views > 0}
          <!-- Abbreviated only because the rail shares its width with a company name that
             truncates, and a five-digit figure spelled out crowds the name off the card.
             A screen reader has no such constraint, so it hears the exact figure — the
             same one the job's own page shows. -->
          <span class="flex items-center gap-1 text-xs tabular-nums">
            <Eye class="size-3.5" aria-hidden="true" />
            <span aria-hidden="true">{formatCount(views)}</span>
            <span class="sr-only">{views} views</span>
          </span>
        {/if}
        {#if posted && !hideCompany}
          <span class={['text-xs tabular-nums', touchActions && 'pointer-coarse:hidden']}>{posted}</span>
        {/if}
        {#if job.application_deadline}
          <span class="whitespace-nowrap text-xs tabular-nums">
            Apply by <time datetime={job.application_deadline}>{job.application_deadline}</time>
          </span>
        {/if}
      </div>
    </div>

    <!-- The title is the card's hero — a size up from the body with tight leading, so
       the eye lands on the role first.

       `lang` sits on the card rather than anywhere above it because one listing
       can hold a Russian, a German and an English title in a row. -->
    <h3
      lang={foreignContentLang(job)}
      class={[
        'font-semibold leading-snug tracking-tight',
        compact ? 'mt-2 line-clamp-1 text-base' : 'line-clamp-2 text-base sm:text-lg',
        !compact && !hideCompany && 'mt-1.5',
        // With the rail hidden the title is the top line, so it reserves the corner controls' room the rail would have.
        hideCompany && (!compact && onHide ? 'pr-20' : 'pr-11'),
        hideCompany && touchActions && 'pointer-coarse:pr-30',
      ]}
    >
      {job.title}
    </h3>

    <!-- Signal row: reality chip + the region/employment facets, grouped under the
       title as quiet outline chips so they read as metadata, not decoration. -->
    <!-- ai_interview_reports belongs in this guard on its own: a card can carry the
       label while having no reality verdict, no freshness, no tags, no countries and no
       credentials, and the row would then be suppressed with the badge inside it. -->
    {#if reality || freshness.length > 0 || metaFacts.length > 0 || credentials.length > 0 || job.ai_interview_reports || directApply || appliedToCompany || equity || questions || salary || (hideCompany && (posted || views > 0 || isViewed || job.application_deadline)) || touchPosted}
      <div class="mt-2 flex flex-wrap items-center gap-1.5">
        <!-- evergreen_posting IS the reality verdict, so showing both chips states one
           fact twice, the second time louder. The ghost chip carries it inside its
           checklist; where ghost is silent, reality renders exactly as before. -->
        {#if directApply}
          <DirectApplyBadge />
        {/if}
        <!-- A fact about the reader, not the posting: they applied to this employer before.
           Ahead of the posting's own signals because it changes what the card is for —
           a second application to the same company is a different decision. -->
        {#if appliedToCompany}
          <AppliedCompanyBadge />
        {/if}
        {#if supersedesReality(ghost, locale())}
          <GhostBadge {ghost} />
        {:else}
          <RealityBadge {reality} plain />
        {/if}
        <!-- A separate fact from the two above: those describe the POSTING's reality, this
           describes how the EMPLOYER screens. It is neutral and joins them rather than
           superseding either. Renders nothing when nobody has reported. -->
        <AIInterviewBadge count={job.ai_interview_reports} />
        <!-- Freshness before the facets: "New" is a fact about right now, the facets are
           stable attributes of the role. Both sit behind the reality/ghost chip, because
           a warning that a posting may not be real outranks a note that it is fresh.
           The badge's title is the claim's own basis (when it was posted), not a
           restatement of its label. -->
        {#each freshness as badge (badge.label)}
          <!-- The tooltip rides a wrapper, not the Badge: the primitive takes only
             variant/class/children, so a `title` passed to it would be dropped
             silently and the badge would state a claim with its evidence gone. The
             wrapper (rather than a span inside) also makes the badge's own padding
             hoverable — the same shape the job's own page uses. -->
          <span title={badge.tooltip} class="inline-flex shrink-0">
            <!-- Plain text, not a pill and not the accent: it says the least on the row, so it
               must not be its loudest shape, and the brand colour is kept for primary actions. -->
            <span class="text-xs font-semibold text-foreground">{badge.label}</span>
          </span>
        {/each}
        <!-- The role's facets, then the pay and the cost of applying, as one line of quiet
           text joined by dots rather than a row of outlined chips: the skills below are
           chips already, and two rows of identical outlines made every card noisy.
           Indexed keys: a tag's text could equal one of the trailing facts. -->
        {#if metaFacts.length || salary || touchPosted || (hideCompany && isViewed)}
          <!-- Inline flow, not a flex row: it wraps between facts the way a sentence does. Each
               fact is an inline-block carrying the dot that follows it, so a wrapped line ends
               on "·" and the next one starts on a fact — a flex row left the dots as items of
               their own, free to open a line. The gap is a margin, not a space: Svelte trims
               whitespace at an {#each} boundary, which is how "views ·Remote" ran together.
               The opening dot is a flex item of its own, so when the whole line wraps under
               the badges it stays behind at the end of theirs; when the touch-only date is all
               the line holds, it is touch-only too, or a desktop card ends on "New ·". -->
          {#if metaLeadDot}<span
              class={[
                'text-xs text-muted-foreground',
                !(metaFacts.length || salary || (hideCompany && isViewed)) && 'hidden pointer-coarse:inline',
              ]}
              aria-hidden="true">·</span
            >{/if}
          <span class="min-w-0 text-xs font-medium text-muted-foreground">
            <!-- The rail's viewed mark, for the employer's page where the rail is not drawn. -->
            {#if hideCompany && isViewed}<Check
                class="mr-1 inline size-3.5 align-text-bottom"
                aria-label="You have viewed this"
              />{/if}
            {#if touchPosted}
              <span class="mr-1 hidden whitespace-nowrap tabular-nums pointer-coarse:inline-block"
                >{touchPosted}{#if metaFacts.length || salary}<span aria-hidden="true">&nbsp;·</span>{/if}</span
              >
            {/if}
            {#each metaFacts as fact, i (i)}
              <span class="mr-1 inline-block whitespace-nowrap"
                >{fact}{#if i < metaFacts.length - 1 || salary}<span aria-hidden="true">&nbsp;·</span>{/if}</span
              >
            {/each}
            <!-- The pay leads into the same line, in ink, rather than standing alone in bold at
               the card's foot, where cards with and without it ended at different heights. -->
            {#if salary}
              <span class="inline-block whitespace-nowrap font-semibold tabular-nums text-foreground">{salary}</span>
            {/if}
          </span>
        {/if}
        <!-- Register-backed employer credentials (visa-sponsor licences). A fact about
           the employer, so it sits with the metadata chips and carries its own
           disclaimer rather than reading as a promise about this role. -->
        <!-- Not on the employer's own page, whose header already states it for every row. -->
        {#if !hideCompany}<CredentialBadge collections={job.collections} plain />{/if}
      </div>
    {/if}

    <!-- The geography line: arrangement, then the flags, then the city the flags do not name. -->
    {#if geoTags.length || job.countries?.length || place}
      <div class="mt-1.5 flex flex-wrap items-center gap-x-1.5 gap-y-1 text-xs font-medium text-muted-foreground">
        {#each geoTags as tag, i (tag)}
          {#if i > 0}<span aria-hidden="true">·</span>{/if}
          <span>{tag}</span>
        {/each}
        <!-- Eligible countries as an overlapping flag cluster. Display-only here: the
           whole card is a link, so the flags carry no nested filter links. -->
        {#if job.countries?.length}
          <CountryFlagStack
            codes={job.countries}
            max={MAX_FLAGS}
            class={geoTags.length ? 'ml-1 text-base' : 'text-base'}
          />
        {/if}
        <!-- The place the flags do not name: one employer's postings in seven cities
           otherwise read identically until opened. -->
        {#if place}
          <span title={job.location} class="flex min-w-0 max-w-56 items-center gap-1">
            <MapPin class="size-3.5 shrink-0" aria-hidden="true" />
            <span class="truncate">{place}</span>
          </span>
        {/if}
      </div>
    {/if}

    {#if blurb && !compact}
      <p class="mt-2 line-clamp-2 text-sm text-muted-foreground">{blurb}</p>
    {/if}

    <!-- The skills, as the card's one chip row (the pay rides the meta line above). -->
    {#if skills.length}
      <div class="mt-3 flex min-w-0 flex-wrap items-center gap-1.5">
        <!-- A long dictionary slug ("event-driven-architecture") would wrap inside its
           own chip and stretch the row to two lines, breaking the card's rhythm on a
           phone. Each chip stays one line and truncates with an ellipsis past max-w; the full skill is
           in `title` for anyone who needs it. -->
        <!-- The label, not the slug: a card read "ci-cd" where the filter panel beside it
           read "CI/CD". No definition here, unlike the chips on the posting itself —
           these sit inside the card's navigation <a>, where a second activation target
           is invalid markup and the hazard the comment at the top of this file is
           about. The card is for scanning; the posting is where you read. -->
        {#each shownSkills as skill (skill)}
          <Badge variant={chipVariant(skill)} class="max-w-[9rem] gap-1">
            <SkillIcon slug={skill} />
            <span class="truncate" title={skillLabel(skill)}>{skillLabel(skill)}</span>
          </Badge>
        {/each}
        {#if extraSkills > 0}
          <span class="text-xs text-muted-foreground">+{extraSkills} skills</span>
        {/if}
      </div>
    {/if}

    <!-- Card-level profile match: the real client-computed coverage bar, only for a
       signed-in viewer with a skills profile. -->
    <JobMatchBar {match} />
    {#if teaser && compact}
      <!-- The compact card has no action row, so its invitation stays a caption inside the
         link. `aria-hidden`: the sr-only span below, outside the <a>, says it instead,
         so it does not join the link's accessible name. -->
      <div class="mt-1.5 flex items-center gap-1.5 text-xs text-muted-foreground" aria-hidden="true">
        {#if matchState === 'guest'}
          <Lock class="size-3.5 shrink-0" />
        {:else}
          <FileText class="size-3.5 shrink-0" />
        {/if}
        {matchInvite}
      </div>
    {/if}
  </a>

  <!-- The card link's description (see `summary`). `hidden` keeps it off the page and out
     of browse mode, where the same facts are read from the card itself; a description
     reference still reaches hidden content. -->
  {#if summary}
    <span id={summaryId} hidden>{summary}</span>
  {/if}

  {#if teaser && compact}
    <!-- Stands in for the compact card's aria-hidden caption. Outside the card link on
       purpose: `sr-only` is clip-based, so inside the <a> it would join the link's name. -->
    <span class="sr-only">{matchInvite}</span>
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
      onclick={toggleSave}
      disabled={saving}
      aria-pressed={saved}
      aria-label={saved ? 'Remove from your board' : 'Save to your board'}
      title={saved ? 'On your board' : 'Save to board'}
      class={[
        'absolute right-2.5 top-4 grid size-8 place-items-center rounded-lg transition hover:bg-accent hover:text-brand disabled:pointer-events-none disabled:opacity-50',
        saved ? 'text-brand' : 'text-muted-foreground',
      ]}
    >
      <Bookmark class="size-[1.05rem] {saved ? 'fill-current' : ''}" aria-hidden="true" />
    </button>
  {:else}
    <!-- The card's controls ride its top-right corner, beside the date, rather than a row of
       their own at its foot: that row added ~40px to every card in a feed of dozens. Save
       is a bookmark glyph with its title and accessible name; Apply and
       Hide, where the card has them, hover-reveal beside it (over the date, which they cover
       only while the pointer is on the card). `pointer-coarse`
       keeps Hide visible on touch, which has no hover to reveal anything; there Apply shrinks
       to its icon and the date moves to the meta line, so the company name is not cut to
       two letters. The rail inside the link reserves the corner (see its `pr-*`), so the
       date never slides under it. -->
    <div class="absolute right-3 top-3 flex items-center gap-1">
      <!-- Direct-apply shortcut (freehire#2755): a way out to the posting's own site
         without opening the job page first. Outline, never the brand-green primary —
         that loud slot is reserved for auto-apply on the job page itself, and a list
         card outshouting the page it links to states the wrong priority. `bg-transparent`
         drops the outline variant's default `bg-background` fill: unlike a lone button on
         a page, this one sits on the card's own background, and that fill read as a solid
         chip louder than every other control on the card — Save carries the same border
         with no fill, and Apply should read as its equal, not its loudest element.
         Hover-reveal matches Hide; `pointer-coarse` is load-bearing, not decoration — a
         phone has no hover, so without it the button is invisible to every touch visitor.
         `nofollow`: the destination is the poster's own site, which the catalogue never
         vetted (the same stance JobView.svelte's own Apply link takes) — without it a
         submitted vacancy buys a followed link from every card in every list. -->
      {#if directApply}
        <Button
          variant="outline"
          href={resolve('/jobs/[slug]', { slug: job.public_slug })}
          aria-label="Apply to {job.title}"
          class="h-8 shrink-0 bg-transparent opacity-0 focus-visible:opacity-100 group-hover:opacity-100 pointer-coarse:w-8 pointer-coarse:px-0 pointer-coarse:opacity-100"
        >
          <FileText class="hidden size-4 pointer-coarse:block" aria-hidden="true" />
          <span class="pointer-coarse:sr-only">Apply</span>
        </Button>
      {/if}
      {#if applyJob}
        <Button
          variant="outline"
          href={isAuthenticated() ? applyJob.url : undefined}
          target="_blank"
          rel="nofollow noopener noreferrer"
          onclick={onApplyClick}
          aria-label={isAuthenticated()
            ? `Apply to ${job.title} on the employer's site (opens in a new tab)`
            : `Choose how to apply to ${job.title}`}
          class="h-8 shrink-0 bg-transparent opacity-0 focus-visible:opacity-100 group-hover:opacity-100 pointer-coarse:w-8 pointer-coarse:px-0 pointer-coarse:opacity-100"
        >
          <ExternalLink class="size-4" aria-hidden="true" />
          <span class="pointer-coarse:sr-only">Apply</span>
        </Button>
      {/if}

      {#if onHide}
        <button
          type="button"
          onclick={hide}
          disabled={hiding}
          aria-label="Hide this job"
          title="Not interested — hide this job"
          class="grid size-8 place-items-center rounded-lg border border-transparent text-muted-foreground opacity-0 transition hover:border-border hover:bg-accent hover:text-foreground focus-visible:opacity-100 group-hover:opacity-100 pointer-coarse:opacity-100 disabled:pointer-events-none disabled:opacity-50"
        >
          <EyeOff class="size-3.5" aria-hidden="true" />
        </button>
      {/if}
      <!-- No `aria-pressed`, unlike the compact glyph: this button's visible name already
         changes with the state, so the toggle role would announce the same fact twice.
         "Saved" is a state, though, and on its own leaves a screen reader user with a
         button whose effect is unstated — so each state's name says what activating does,
         and keeps the visible word in it. -->
      <button
        type="button"
        onclick={toggleSave}
        disabled={saving}
        aria-label={saved ? 'Remove from saved' : 'Save to board'}
        title={saved ? 'On your board' : 'Save to board'}
        class={[
          'grid size-8 shrink-0 place-items-center rounded-lg transition disabled:pointer-events-none disabled:opacity-50',
          saved
            ? 'bg-brand/10 text-brand hover:bg-brand/15'
            : 'text-muted-foreground hover:bg-background hover:text-foreground',
        ]}
      >
        <Bookmark class="size-4 {saved ? 'fill-current' : ''}" aria-hidden="true" />
        <!-- Icon only, with its title and aria-label carrying the action: twenty labelled Save
           buttons down a feed were repeated chrome beside each title. -->
        <span class="sr-only">{saved ? 'Saved' : 'Save'}</span>
      </button>
    </div>

    <!-- The caller's own controls, when it passes any, get the one row under the content. -->
    {#if footer}
      <div class="-mt-1 flex items-center gap-2 px-4 pb-3">
        {@render footer()}
      </div>
    {/if}
  {/if}

  {#if justSaved && saved && !compact}
    <!-- The same confirmation the job page shows after applying: saving puts the job on
       the tracking board, and this is the way there while the click is still fresh.
       Not in `compact`, where the chat column has no room for a second row. -->
    <div
      class="relative z-10 mx-4 mb-3 flex flex-wrap items-center justify-between gap-2 rounded-lg border border-brand/30 bg-brand-muted px-3 py-2 text-sm"
    >
      <span class="inline-flex items-center gap-1.5 font-medium text-brand-strong">
        <Check class="size-4 shrink-0" aria-hidden="true" /> Saved to your board
      </span>
      <a
        href={resolve('/my/tracking/[id]', { id: job.public_slug })}
        class="font-medium text-brand-strong underline underline-offset-4"
      >
        Open in tracker →
      </a>
    </div>
  {/if}

  <div class="sr-only" aria-live="polite" aria-atomic="true">
    {#if showApplyGate && applyJob}
      Choose how to apply to {job.title}: sign in, sign up, or continue without signing in.
    {/if}
  </div>

  {#if showApplyGate && applyJob}
    <div
      class="relative z-10 mx-4 mb-3 flex flex-col gap-3 rounded-lg border border-border bg-secondary px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
    >
      <span class="text-sm text-pretty">Sign in to keep track of the jobs you apply to.</span>
      <!-- Two equal buttons over a full-width escape on a phone, where three in a wrapping
         row left the last one stranded on a line of its own; one row from sm up. -->
      <div class="grid grid-cols-2 gap-2 sm:flex sm:shrink-0 sm:items-center">
        <Button variant="primary" onclick={signInFromApplyGate}>Sign in</Button>
        <Button variant="outline" onclick={signUpFromApplyGate}>Sign up</Button>
        <Button variant="ghost" class="col-span-2" onclick={viewApplyWithoutSigningIn}>
          Continue without signing in
        </Button>
      </div>
    </div>
  {/if}
</div>
