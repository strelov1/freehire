<script lang="ts">
  import { page } from '$app/state';
  import { ApiError, api } from '$lib/api';
  import type { TalentNetworkVisibility } from '$lib/types';
  import { Button } from '$lib/ui';

  // The Talent Network opt-in: Off (default) / Public / Anonymous, plus a preview of the
  // resulting shareable URL. The public page itself is a later, separate piece of work —
  // this only reads/writes the setting via GET/PUT /me/talent-network and shows what the
  // link would be, built from the same path the backend's public route is mounted at
  // (see internal/handler/talent_network_profile.go's register: "/talent-network/:publicID",
  // sibling to /api/v1, not under it — see handler.go).

  let { onError }: { onError: (message: string | null) => void } = $props();

  const OPTIONS: { id: TalentNetworkVisibility; label: string; description: string }[] = [
    { id: 'off', label: 'Off', description: 'Hidden. No public link resolves.' },
    { id: 'public', label: 'Public', description: 'Shows your name, work history and skills.' },
    {
      id: 'anonymous',
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
  let copied = $state(false);

  $effect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const setting = await api.getTalentNetwork();
        if (cancelled) return;
        visibility = setting.talent_network_visibility;
        publicId = setting.talent_network_public_id;
        status = 'ready';
      } catch (e) {
        if (cancelled) return;
        status = 'error';
        onError(
          e instanceof ApiError ? e.message : 'Could not load your Talent Network setting.',
        );
      }
    })();
    return () => {
      cancelled = true;
    };
  });

  const publicUrl = $derived(`${page.url.origin}/talent-network/${publicId}`);

  async function select(next: TalentNetworkVisibility) {
    if (next === visibility || saving) return;
    const previous = visibility;
    saving = true;
    onError(null);
    try {
      // Trust the echoed value, not the click — a rejected PUT (e.g. a bad value some
      // future caller sends) must not leave the picker showing a state that never saved.
      const setting = await api.setTalentNetworkVisibility(next);
      visibility = setting.talent_network_visibility;
      publicId = setting.talent_network_public_id;
    } catch (e) {
      visibility = previous;
      onError(
        e instanceof ApiError ? e.message : 'Could not update your Talent Network setting.',
      );
    } finally {
      saving = false;
    }
  }

  async function copyLink() {
    try {
      await navigator.clipboard.writeText(publicUrl);
      copied = true;
      setTimeout(() => {
        copied = false;
      }, 1500);
    } catch {
      onError('Could not copy the link.');
    }
  }
</script>

<section class="flex flex-col gap-3 rounded-xl border border-border bg-card p-5 sm:p-6">
  <div class="flex flex-col gap-1">
    <h2 class="text-sm font-semibold tracking-tight">Talent Network</h2>
    <p class="text-sm text-muted-foreground">
      Get discovered without applying anywhere: publish a read-only profile page at a
      private link you can share yourself.
    </p>
  </div>

  {#if status === 'loading'}
    <p class="text-sm text-muted-foreground">Loading…</p>
  {:else if status === 'error'}
    <!-- The failure itself is already surfaced above via the page's actionError banner
         (onError) — rendering the picker here too would show it defaulted to "Off",
         which may not reflect the account's real, unknown-because-unloaded state. -->
    <p class="text-sm text-muted-foreground">Couldn't load this setting.</p>
  {:else}
    <div role="radiogroup" aria-label="Talent Network visibility" class="grid gap-2 sm:grid-cols-3">
      {#each OPTIONS as opt (opt.id)}
        <button
          type="button"
          role="radio"
          aria-checked={opt.id === visibility}
          disabled={saving}
          onclick={() => select(opt.id)}
          class={[
            'flex flex-col gap-0.5 rounded-lg border px-3 py-2.5 text-left transition-colors disabled:opacity-60',
            opt.id === visibility
              ? 'border-primary ring-2 ring-primary/40'
              : 'border-border hover:border-foreground/40',
          ]}
        >
          <span class="text-sm font-medium text-foreground">{opt.label}</span>
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

    {#if visibility !== 'off'}
      <!-- Not an <a>: the public page itself isn't wired up yet (a separate, later
           piece of work) — this previews the link a candidate would get without
           routing anywhere. -->
      <div class="flex flex-wrap items-center gap-2 rounded-lg bg-secondary/50 px-3 py-2">
        <span class="min-w-0 truncate font-mono text-xs text-foreground">{publicUrl}</span>
        <Button variant="ghost" size="sm" class="ml-auto" onclick={copyLink}>
          {copied ? 'Copied' : 'Copy link'}
        </Button>
      </div>
    {/if}
  {/if}
</section>
