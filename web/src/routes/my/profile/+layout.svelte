<script lang="ts">
  import type { Snippet } from 'svelte';
  import { Globe, VenetianMask } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { api } from '$lib/api';
  import { currentUser, isAuthenticated } from '$lib/auth.svelte';
  import { routeTabClass, tablist } from '$lib/actions/tablist';
  import AccountPreferences from '$lib/components/AccountPreferences.svelte';
  import DeleteAccountButton from '$lib/components/DeleteAccountButton.svelte';
  import ProfileForm from '$lib/components/ProfileForm.svelte';
  import States from '$lib/components/States.svelte';
  import { profileStore } from '$lib/profile.svelte';
  import { resumeStore } from '$lib/resume.svelte';
  import type { TalentNetworkVisibility } from '$lib/types';
  import { Button } from '$lib/ui';

  let { children }: { children: Snippet } = $props();

  const profile = $derived(profileStore.profile);

  let status = $state<'loading' | 'error' | 'ready'>('loading');

  // Status-aware Talent Network entry button in the page header — links to
  // /my/talent-network. `null` means "not yet loaded" — the button renders in its
  // "off" (join) state until the fetch resolves, same fail-safe posture as a load
  // failure below.
  let talentNetworkVisibility = $state<TalentNetworkVisibility | null>(null);

  // Each section is its own URL, so it is linkable, bookmarkable, and survives a
  // reload. Settings is the index route. CV readiness (/my/profile/cv-readiness) is
  // deliberately ABSENT from this list: the page still works for anyone who holds the
  // link, it just is not offered in the strip.
  const TABS = [
    { id: 'profile-tab-settings', href: '/my/profile', label: 'Settings' },
    { id: 'profile-tab-skills', href: '/my/profile/skills', label: 'Skills' },
    { id: 'profile-tab-contacts', href: '/my/profile/contacts', label: 'Profile' },
    { id: 'profile-tab-experience', href: '/my/profile/experience', label: 'Experience' },
    { id: 'profile-tab-screening', href: '/my/profile/screening', label: 'Screening answers' },
  ] as const;
  const PANEL_ID = 'profile-tabpanel';

  const path = $derived(page.url.pathname);
  // Settings (index) matches exactly so it is not also active on the child routes.
  const isActive = (href: string) => (href === '/my/profile' ? path === href : path.startsWith(href));
  // Undefined on a section with no tab of its own (CV readiness) — there is nothing for
  // the panel to point back at then.
  const activeTabId = $derived(TABS.find((t) => isActive(t.href))?.id);

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

  // Seeds the header button's display only — never blocking, and no error surfaced here.
  // A failure defaults the button to its "off" (join) state; the panel's own fetch (on
  // open) is the informative path if the setting genuinely can't be read.
  async function loadTalentNetwork() {
    try {
      const setting = await api.getTalentNetwork();
      talentNetworkVisibility = setting.talent_network_visibility;
    } catch {
      talentNetworkVisibility = 'off';
    }
  }

  // (Re)load once the session resolves. Talent Network is beta-gated: skip its fetch
  // entirely for a non-beta account, same as the button being hidden for one.
  $effect(() => {
    if (isAuthenticated()) {
      void load();
      if (currentUser()?.beta_tester) void loadTalentNetwork();
    }
  });
</script>

<svelte:head>
  <!-- Base title; each section overrides it with its own name. -->
  <title>Profile — freehire</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex. -->
{#if status === 'loading'}
  <States state="loading" />
{:else if status === 'error'}
  <States state="error" message="Couldn't load your profile." />
{:else}
  <!-- Header -->
  <div class="mb-6 flex flex-col items-start gap-4 sm:flex-row sm:items-start sm:justify-between">
    <div class="flex flex-col gap-1">
      <h1 class="text-2xl font-semibold tracking-tight">Profile</h1>
      <p class="text-sm text-muted-foreground">
        Your CV, skills and role — measured against live market demand.
      </p>
    </div>
    <!-- Off/not-yet-loaded is a filled call-to-action (the opt-in is the interesting
         choice to make); once public or anonymous the button becomes a low-key status
         readout, since Off is the default and the other two are already a
         deliberate, weighty decision the /my/talent-network page itself re-explains.
         Beta-gated: hidden entirely for a non-beta account, not just disabled — the
         feature isn't ready for a general audience yet. -->
    {#if currentUser()?.beta_tester}
      <Button
        variant={talentNetworkVisibility === 'public' || talentNetworkVisibility === 'anonymous'
          ? 'outline'
          : 'primary'}
        class="shrink-0"
        href={resolve('/my/talent-network')}
      >
        {#if talentNetworkVisibility === 'public'}
          <Globe class="size-4" aria-hidden="true" /> Talent Network: Public
        {:else if talentNetworkVisibility === 'anonymous'}
          <VenetianMask class="size-4" aria-hidden="true" /> Talent Network: Anonymous
        {:else}
          Join Talent Network
        {/if}
      </Button>
    {/if}
  </div>

  {#if profile === null}
    <!-- Set-up: the inline form only; the sections appear once a profile exists. Full-width
         to match the loaded profile, whose main column also spans the container. -->
    <div class="w-full">
      <ProfileForm
        profile={null}
        hasCv={resumeStore.present}
        onCvUploaded={() => resumeStore.noteUpload()}
      />
      <AccountPreferences class="mt-6" />
      <!-- Set-up is the settings section before there are sections, so leaving is offered
           here too: someone who signed up, filled in nothing and wants out must not have to
           create a profile first to find the way back out. -->
      <div class="mt-2 flex justify-end border-t border-border pt-4">
        <DeleteAccountButton />
      </div>
    </div>
  {:else}
    <div class="flex flex-col gap-6">
      <div role="tablist" aria-label="Profile sections" use:tablist={path} class="flex flex-wrap items-center gap-1">
        {#each TABS as t (t.href)}
          {@const active = isActive(t.href)}
          <a
            role="tab"
            id={t.id}
            aria-selected={active}
            aria-controls={PANEL_ID}
            href={resolve(t.href)}
            class={routeTabClass(active)}
          >
            {t.label}
          </a>
        {/each}
      </div>

      <div
        id={PANEL_ID}
        role="tabpanel"
        aria-labelledby={activeTabId}
        tabindex="0"
      >
        {@render children()}
      </div>
    </div>
  {/if}
{/if}
