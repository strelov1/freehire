<script lang="ts">
  import { tick, untrack, type Snippet } from 'svelte';
  import { scrollBehavior } from '#lib/motion.js';
  import { Briefcase, ClipboardList, Tags, User } from '@lucide/svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { Badge, TabStrip, tabStripId } from '#lib/ui/index.js';
  import { activeRouteTab } from '#lib/routeTabs.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';
  import { profileCompleteness } from '#lib/profileCompleteness.svelte.js';
  import type { CompletenessSection } from '#lib/profileCompleteness.js';
  import { resumeStore } from '#lib/resume.svelte.js';
  import { experienceBankStore } from '#lib/experienceBank.svelte.js';
  import { screeningStore } from '#lib/screening.svelte.js';
  import AccountPreferences from '#lib/components/AccountPreferences.svelte';
  import AccountSetupCard from '#lib/components/AccountSetupCard.svelte';
  import ProfileForm from '#lib/components/ProfileForm.svelte';
  import States from '#lib/components/States.svelte';
  import { profileStore } from '#lib/profile.svelte.js';
  import { handleCvDeleted, handleCvUploaded, handleSaved } from './actions';
  import { messages } from './messages';

  // The account shell (my/+layout) owns the container, auth gate, and noindex; this
  // layout adds Profile's own section navigation and the shared load gate. Each
  // section is its own route so it is linkable, bookmarkable, and survives a reload —
  // see docs/superpowers/specs/2026-09-04-profile-section-routes-design.md. Before a
  // profile exists there is nothing to navigate between, so the tab strip (and
  // `children`) stay hidden on every one of these routes, same as the old single-page
  // version.

  let { children }: { children: Snippet } = $props();

  // Icons match the account sidebar's convention (see accountNavIcons.ts) so the row
  // scans the same way collapsed; `TabStrip` owns the overflow behaviour on a narrow
  // viewport. CV readiness is deliberately not one of these — it's not
  // published yet; it still works at /my/profile/cv-readiness for anyone who holds
  // the link, nothing here links to it.
  //
  // Four tabs, each holding its neighbours: Profile carries Contacts and Location (with the
  // timezone), Skills carries Avoid, Experience carries Education. The old routes of the
  // absorbed sections redirect to their anchors. There is no Settings section: the
  // timezone sits in Location and the language on /my/security (see
  // profile/settings/+page.ts).
  const SECTIONS = [
    { id: 'profile', href: '/my/profile', icon: User },
    { id: 'skills', href: '/my/profile/skills', icon: Tags },
    { id: 'experience', href: '/my/profile/experience', icon: Briefcase },
    { id: 'screening', href: '/my/profile/screening', icon: ClipboardList },
  ] as const;

  const PANEL_ID = 'profile-panel';

  // The counted sections each tab holds. A tab with any of them still empty carries a dot;
  // a filled one carries nothing, since "done" needs no announcement.
  const TAB_SECTIONS: Record<(typeof SECTIONS)[number]['id'], readonly CompletenessSection[]> = {
    profile: ['summary', 'contacts'],
    skills: ['skills'],
    experience: ['experience', 'education'],
    screening: ['screening'],
  };

  const s = $derived(t(messages, locale()));
  const profile = $derived(profileStore.profile);
  const resumeMeta = $derived(resumeStore.meta);
  const completeness = $derived(profileCompleteness());
  const activeSectionId = $derived(activeRouteTab(page.url.pathname, SECTIONS, 'profile'));
  const tabs = $derived(
    SECTIONS.map((sec) => ({
      ...sec,
      label: s.tabs[sec.id],
      href: resolve(sec.href),
      attention:
        completeness && TAB_SECTIONS[sec.id].some((section) => !completeness[section])
          ? s.completeness.incomplete
          : undefined,
    })),
  );

  let status = $state<'loading' | 'error' | 'ready'>('loading');

  async function load() {
    status = 'loading';
    try {
      await profileStore.ensureLoaded();
      status = 'ready';
    } catch {
      status = 'error';
    }
    void resumeStore.ensureLoaded();
    void screeningStore.ensureLoaded();
    void experienceBankStore.ensureLoaded();
  }

  // (Re)load once the session resolves.
  $effect(() => {
    if (isAuthenticated()) void load();
  });

  // Honour the URL's anchor once the gate opens. The browser and the router both resolve a
  // `#id` at navigation time, and until `status` is 'ready' this layout renders a spinner —
  // so a RELOAD or a shared link to /my/profile#account-cv would consume the anchor against
  // nothing and land at the top, which is exactly the "clicking it does nothing" this card's
  // anchors exist to fix. Following a link from inside the app is unaffected: the tree is
  // already up, so this fires on a target the browser has just scrolled to anyway.
  //
  // Runs once per hash, not per render: `scrolledToHash` is compared before scrolling, so an
  // unrelated profile refresh cannot yank the reader back to an anchor they scrolled away
  // from. `untrack` keeps that read from making this effect its own trigger.
  let scrolledToHash = $state<string | null>(null);
  $effect(() => {
    const hash = page.url.hash.slice(1);
    if (status !== 'ready' || !hash || hash === untrack(() => scrolledToHash)) return;
    let release: (() => void) | undefined;
    // The section that owns the anchor renders in the same tick this effect runs, so wait
    // for the DOM to catch up rather than querying a target that does not exist yet.
    void tick().then(() => {
      const target = document.getElementById(hash);
      if (!target) return;
      scrolledToHash = hash;
      release = scrollAndHold(target);
    });
    return () => release?.();
  });

  // The sections load their own data after the gate opens, so the page is still growing when
  // the first scroll is computed — and a scroll clamped to the shorter page stops short of
  // its target (measured: /my/profile#account-location stopped ~770px short). So scroll
  // again whenever the target moves on the page or the page's height changes, until the
  // reader scrolls on their own or a few seconds pass. Checked per frame rather than with a
  // ResizeObserver: the body is pinned to the window's height and the content overflows
  // it, so no single box both grows and covers what loads above the target. Returns the
  // release, for when the anchor or the page goes away first.
  const HOLD_MS = 3000;
  const READER_SCROLLS = ['wheel', 'touchstart', 'keydown', 'pointerdown'] as const;
  function scrollAndHold(target: HTMLElement): () => void {
    const until = performance.now() + HOLD_MS;
    let last = '';
    let frame = requestAnimationFrame(function check() {
      const place = `${target.getBoundingClientRect().top + scrollY}|${document.documentElement.scrollHeight}`;
      if (place !== last) {
        last = place;
        target.scrollIntoView({ behavior: scrollBehavior(), block: 'start' });
      }
      if (performance.now() < until) frame = requestAnimationFrame(check);
      else release();
    });
    for (const type of READER_SCROLLS) addEventListener(type, release, { passive: true });
    function release() {
      cancelAnimationFrame(frame);
      for (const type of READER_SCROLLS) removeEventListener(type, release);
    }
    return release;
  }
</script>

<svelte:head>
  <title>Profile — freehire</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex. The
     heading sits above the gate so the section is titled while it is still loading and
     whether or not a profile exists yet, the way every other /my section titles itself. -->
<div class="mb-6 flex flex-col gap-1 max-lg:sr-only">
  <h1 class="text-2xl font-semibold tracking-tight">{s.title}</h1>
  <p class="text-sm text-muted-foreground">{s.description}</p>
</div>

{#if status === 'loading'}
  <States state="loading" />
{:else if status === 'error'}
  <States state="error" message={s.loadError} onRetry={() => void load()} />
{:else if profile === null}
  <!-- Set-up: the inline form only; coverage appears once a profile exists. -->
  <div class="w-full">
    <ProfileForm
      profile={null}
      hasCv={resumeStore.present}
      uploadedAt={resumeMeta?.uploaded_at}
      onSaved={handleSaved}
      onCvUploaded={handleCvUploaded}
      onCvDeleted={handleCvDeleted}
    />
    <!-- Account-level (not candidate-profile) settings live here too, so they never wait on
         a CV upload; once a profile exists they move to Profile's Location part and /my/security. -->
    <AccountPreferences class="mt-6" />
  </div>
{:else}
  <!-- Above the tab strip: what is left to set up belongs to the account rather than to
       whichever section happens to be open — a card inside a section would be
       re-announced on every tab switch. The Talent Network invitation used to sit beside
       it and has moved further out still, to the account shell (`my/+layout`), which puts
       it above every section rather than only above Profile's. -->
  <div class="mb-6">
    <AccountSetupCard />
  </div>

  <!-- The overall count reads the same helper the tab dots do, so the two cannot
       disagree. Shown only while something is missing — a finished profile needs no
       scoreboard. It sits above the strip rather than inside it: the strip is navigation,
       and this is a measurement of the whole profile. -->
  {#if completeness && completeness.filled < completeness.total}
    <div class="mb-3 flex justify-end">
      <Badge variant="outline" class="tabular-nums">
        {format(s.completeness.summary, {
          filled: String(completeness.filled),
          total: String(completeness.total),
        })}
      </Badge>
    </div>
  {/if}

  <TabStrip
    {tabs}
    active={activeSectionId}
    label="Profile sections"
    panelId={PANEL_ID}
    class="mb-6"
  />

  <div
    id={PANEL_ID}
    role="tabpanel"
    aria-labelledby={tabStripId(PANEL_ID, activeSectionId)}
    tabindex="0"
  >
    {@render children()}
  </div>
{/if}
