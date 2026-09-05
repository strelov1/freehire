<script lang="ts">
  import { EyeOff, Globe, VenetianMask } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { ApiError, api } from '$lib/api';
  import type { TalentNetworkVisibility } from '$lib/types';
  import { Button } from '$lib/ui';

  // The Talent Network opt-in: Off (default) / Public / Anonymous, plus a way to view
  // the resulting shareable public page. A dedicated page, not an overlay — reached from
  // the entry button on my/profile. Same GET/PUT /me/talent-network contract as before.

  const OPTIONS: {
    id: TalentNetworkVisibility;
    icon: typeof EyeOff;
    label: string;
    description: string;
  }[] = [
    { id: 'off', icon: EyeOff, label: 'Off', description: 'Hidden. No public link resolves.' },
    {
      id: 'public',
      icon: Globe,
      label: 'Public',
      description: 'Shows your name, work history and skills.',
    },
    {
      id: 'anonymous',
      icon: VenetianMask,
      label: 'Anonymous',
      description: 'Hides your name; masks your current employer.',
    },
  ];

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  let visibility = $state<TalentNetworkVisibility>('off');
  let publicId = $state('');
  // Disables the picker while a change is in flight, so a fast double-click can't race
  // two PUTs (same guard TemplateGallery uses for its own exclusive-choice picker).
  let saving = $state(false);
  let saveError = $state<string | null>(null);

  $effect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const setting = await api.getTalentNetwork();
        if (cancelled) return;
        visibility = setting.talent_network_visibility;
        publicId = setting.talent_network_public_id;
        status = 'ready';
      } catch {
        if (cancelled) return;
        status = 'error';
      }
    })();
    return () => {
      cancelled = true;
    };
  });

  async function select(next: TalentNetworkVisibility) {
    if (next === visibility || saving) return;
    const previous = visibility;
    saving = true;
    saveError = null;
    try {
      // Trust the echoed value, not the click — a rejected PUT must not leave the
      // picker showing a state that never saved.
      const setting = await api.setTalentNetworkVisibility(next);
      visibility = setting.talent_network_visibility;
      publicId = setting.talent_network_public_id;
    } catch (e) {
      visibility = previous;
      saveError =
        e instanceof ApiError ? e.message : 'Could not update your Talent Network setting.';
    } finally {
      saving = false;
    }
  }
</script>

<svelte:head>
  <title>Talent Network — freehire</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex;
     an inner max-width keeps the content readable within the content column. -->
<div class="flex max-w-2xl flex-col gap-4">
  <div class="flex flex-col items-start gap-4 sm:flex-row sm:items-start sm:justify-between">
    <div class="flex flex-col gap-1">
      <h1 class="text-2xl font-semibold tracking-tight">Talent Network</h1>
      <p class="text-sm text-muted-foreground">
        Get discovered without applying anywhere: publish a read-only profile page you control.
      </p>
    </div>
    {#if status === 'ready' && visibility !== 'off'}
      <!-- Opens the actual public page in a new tab, so a candidate can see exactly
           what a recruiter would see. No raw URL shown and no copy action here — a
           visitor can copy it from the browser's own address bar once there. -->
      <Button
        variant="primary"
        class="shrink-0"
        href={resolve('/talent-network/[publicId]', { publicId })}
        target="_blank"
        rel="noopener noreferrer"
      >
        View your public page
      </Button>
    {/if}
  </div>

  {#if status === 'loading'}
    <p class="text-sm text-muted-foreground">Loading…</p>
  {:else if status === 'error'}
    <p class="text-sm text-muted-foreground">Couldn't load this setting.</p>
  {:else}
    {#if saveError}
      <p class="text-sm text-destructive">{saveError}</p>
    {/if}

    <div class="grid gap-2 sm:grid-cols-3">
      {#each OPTIONS as opt (opt.id)}
        {@const Icon = opt.icon}
        <button
          type="button"
          aria-pressed={opt.id === visibility}
          disabled={saving}
          onclick={() => select(opt.id)}
          class={[
            'flex flex-col gap-0.5 rounded-lg border px-3 py-2.5 text-left transition-colors disabled:opacity-60',
            opt.id === visibility
              ? 'border-primary ring-2 ring-primary/40'
              : 'border-border hover:border-foreground/40',
          ]}
        >
          <span class="flex items-center gap-1.5 text-sm font-medium text-foreground">
            <Icon class="size-4" aria-hidden="true" />
            {opt.label}
          </span>
          <span class="text-xs text-muted-foreground">{opt.description}</span>
        </button>
      {/each}
    </div>

    <!-- Visible regardless of the current mode, not just once shared — the trade-off
         needs to be read before a candidate opts in, not discovered afterward. -->
    <p class="text-xs text-muted-foreground">
      Once you share this link, anyone who has it may have already seen or saved your
      profile — switching back to Off removes the live page but can't undo that.
    </p>
  {/if}
</div>
