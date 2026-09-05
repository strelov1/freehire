<script lang="ts">
  import { tick, untrack, type Snippet } from 'svelte';
  import {
    Briefcase,
    ClipboardList,
    Contact,
    GraduationCap,
    MapPin,
    Settings as SettingsIcon,
    Tags,
    User,
  } from '@lucide/svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { TabStrip, tabStripId } from '$lib/ui';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { t } from '$lib/i18n/t';
  import { resumeStore } from '$lib/resume.svelte';
  import AccountPreferences from '$lib/components/AccountPreferences.svelte';
  import AccountSetupCard from '$lib/components/AccountSetupCard.svelte';
  import ProfileForm from '$lib/components/ProfileForm.svelte';
  import States from '$lib/components/States.svelte';
  import { profileStore } from '$lib/profile.svelte';
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
  const SECTIONS = [
    { id: 'profile', href: '/my/profile', icon: User },
    { id: 'contacts', href: '/my/profile/contacts', icon: Contact },
    { id: 'location', href: '/my/profile/location', icon: MapPin },
    { id: 'skills', href: '/my/profile/skills', icon: Tags },
    { id: 'experience', href: '/my/profile/experience', icon: Briefcase },
    { id: 'education', href: '/my/profile/education', icon: GraduationCap },
    { id: 'screening', href: '/my/profile/screening', icon: ClipboardList },
    { id: 'settings', href: '/my/profile/settings', icon: SettingsIcon },
  ] as const;

  const PANEL_ID = 'profile-panel';

  const s = $derived(t(messages, locale()));
  const profile = $derived(profileStore.profile);
  const resumeMeta = $derived(resumeStore.meta);
  const path = $derived(page.url.pathname);
  const activeSectionId = $derived(SECTIONS.find((sec) => sec.href === path)?.id ?? 'profile');
  const tabs = $derived(
    SECTIONS.map((sec) => ({
      id: sec.id,
      label: s.tabs[sec.id],
      icon: sec.icon,
      href: resolve(sec.href),
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
    // The section that owns the anchor renders in the same tick this effect runs, so wait
    // for the DOM to catch up rather than querying a target that does not exist yet.
    void tick().then(() => {
      const target = document.getElementById(hash);
      if (!target) return;
      scrolledToHash = hash;
      target.scrollIntoView({ behavior: 'smooth', block: 'start' });
    });
  });
</script>

<svelte:head>
  <title>Profile — freehire</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex. -->
{#if status === 'loading'}
  <States state="loading" />
{:else if status === 'error'}
  <States state="error" message="Couldn't load your profile." />
{:else if profile === null}
  <!-- Set-up: the inline form only; coverage appears once a profile exists. -->
  <div class="mb-6 flex flex-col gap-1">
    <h1 class="text-2xl font-semibold tracking-tight">Profile</h1>
    <p class="text-sm text-muted-foreground">
      Your CV, skills and role — measured against live market demand.
    </p>
  </div>
  <div class="w-full">
    <ProfileForm
      profile={null}
      hasCv={resumeStore.present}
      uploadedAt={resumeMeta?.uploaded_at}
      onSaved={handleSaved}
      onCvUploaded={handleCvUploaded}
      onCvDeleted={handleCvDeleted}
    />
    <!-- Set-up is Settings before there are views, so account-level (not candidate-profile)
         settings live here too rather than waiting on a CV upload. -->
    <AccountPreferences class="mt-6" />
  </div>
{:else}
  <!-- Above the tab strip: what is left to set up belongs to the account, not to
       whichever section happens to be open, and a card inside a section would be
       re-announced on every tab switch. -->
  <div class="mb-6">
    <AccountSetupCard />
  </div>

  <!-- Same heading the set-up branch above renders, so the page does not lose its title
       the moment a profile exists — and so this section is titled like every other
       /my/* one. -->
  <div class="mb-6 flex flex-col gap-1">
    <h1 class="text-2xl font-semibold tracking-tight">Profile</h1>
    <p class="text-sm text-muted-foreground">
      Your CV, skills and role — measured against live market demand.
    </p>
  </div>

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
