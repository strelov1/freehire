<script lang="ts">
  import type { Snippet } from 'svelte';
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
  import { tablist } from '$lib/actions/tablist';
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
  // scans the same way collapsed; the row scrolls horizontally on narrow viewports
  // rather than wrapping, same as the account nav's own mobile strip in
  // my/+layout.svelte. CV readiness is deliberately not one of these — it's not
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

  const s = $derived(t(messages, locale()));
  const profile = $derived(profileStore.profile);
  const resumeMeta = $derived(resumeStore.meta);
  const path = $derived(page.url.pathname);
  const activeSectionId = $derived(SECTIONS.find((sec) => sec.href === path)?.id ?? 'profile');

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

  <!-- Underline tabs, same style as the Inbox page's Inbox/Settings switch. -->
  <div class="mb-6 flex items-end justify-between gap-4 border-b border-border text-sm">
    <div
      class="no-scrollbar flex min-w-0 gap-4 overflow-x-auto"
      role="tablist"
      aria-label="Profile sections"
      use:tablist={path}
    >
      {#each SECTIONS as sec (sec.id)}
        {@const Icon = sec.icon}
        {@const active = path === sec.href}
        <a
          role="tab"
          id="profile-tab-{sec.id}"
          aria-selected={active}
          aria-controls="profile-panel"
          href={resolve(sec.href)}
          class="-mb-px flex shrink-0 items-center gap-1.5 whitespace-nowrap border-b-2 px-1 py-2 transition-colors {active
            ? 'border-brand font-medium text-foreground'
            : 'border-transparent text-muted-foreground hover:text-foreground'}"
        >
          <Icon class="size-4" aria-hidden="true" />
          {s.tabs[sec.id]}
        </a>
      {/each}
    </div>
  </div>

  <div
    id="profile-panel"
    role="tabpanel"
    aria-labelledby="profile-tab-{activeSectionId}"
    tabindex="0"
  >
    {@render children()}
  </div>
{/if}
