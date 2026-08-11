<script lang="ts">
  import { X } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { ApiError, api } from '$lib/api';
  import type { TalentNetworkVisibility } from '$lib/types';
  import { Button } from '$lib/ui';
  import { focusTrap } from '$lib/actions/focusTrap';

  // The overlay entry point for the Talent Network opt-in: Off (default) / Public /
  // Anonymous, plus the resulting shareable public page. Supersedes the now-deleted
  // inline settings-tab version — same GET/PUT /me/talent-network contract, now presented
  // as a dedicated panel rather than a buried settings-tab section, because opting into
  // a public profile is a deliberate, weighty decision. The preview URL is built as
  // origin + "/talent-network/" + the
  // public id, following this codebase's convention that a public-facing page path
  // mirrors its API resource name 1:1 minus the "/api/v1" prefix — e.g.
  // "/api/v1/companies/:slug" -> "/companies/[slug]" and "/api/v1/jobs/:slug" ->
  // "/jobs/[slug]" (internal/handler/companies.go, internal/handler/jobs.go,
  // web/src/routes/companies/[slug], web/src/routes/jobs/[slug]). (Saved-search boards
  // are the one exception to this — "/api/v1/boards/:slug" shortens to "/b/[slug]" — so
  // that one doesn't support the prediction here.)

  let {
    open,
    onClose,
    onChange,
    onError,
  }: {
    open: boolean;
    onClose: () => void;
    /** Fired right after a successful PUT is applied, so the entry button (a sibling,
     *  not a child) can update its own displayed state without a second fetch. */
    onChange: (setting: TalentNetworkVisibility, publicId: string) => void;
    onError: (message: string | null) => void;
  } = $props();

  const OPTIONS: {
    id: TalentNetworkVisibility;
    icon: string;
    label: string;
    description: string;
  }[] = [
    { id: 'off', icon: '🚫', label: 'Off', description: 'Hidden. No public link resolves.' },
    {
      id: 'public',
      icon: '🌐',
      label: 'Public',
      description: 'Shows your name, work history and skills.',
    },
    {
      id: 'anonymous',
      icon: '🕶️',
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
  // The panel's own error surface for the save/copy paths: the panel is a `fixed
  // inset-0` overlay with aria-modal="true", so the parent page's onError banner
  // renders visually behind the backdrop and is hidden from assistive tech entirely
  // while this is open. onError is still called too, so the parent keeps a record for
  // after the panel closes; this is what's actually visible while it's open.
  let localError = $state<string | null>(null);

  // Loads fresh each time the panel opens (the component stays mounted between opens,
  // unlike a route-level dialog) — mirrors the mount-driven fetch the settings-tab
  // version had, since a closed-then-reopened panel should reflect any change made
  // elsewhere in the meantime.
  $effect(() => {
    if (!open) return;
    let cancelled = false;
    status = 'loading';
    localError = null;
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
    localError = null;
    onError(null);
    try {
      // Trust the echoed value, not the click — a rejected PUT (e.g. a bad value some
      // future caller sends) must not leave the picker showing a state that never saved.
      const setting = await api.setTalentNetworkVisibility(next);
      visibility = setting.talent_network_visibility;
      publicId = setting.talent_network_public_id;
      onChange(visibility, publicId);
    } catch (e) {
      visibility = previous;
      const message =
        e instanceof ApiError ? e.message : 'Could not update your Talent Network setting.';
      localError = message;
      onError(message);
    } finally {
      saving = false;
    }
  }

  async function copyLink() {
    try {
      await navigator.clipboard.writeText(publicUrl);
      copied = true;
      localError = null;
      setTimeout(() => {
        copied = false;
      }, 1500);
    } catch {
      localError = 'Could not copy the link.';
      onError('Could not copy the link.');
    }
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose();
  }
</script>

<svelte:window onkeydown={open ? onKeydown : undefined} />

{#if open}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
    <button
      type="button"
      aria-label="Close dialog"
      class="absolute inset-0 bg-black/50"
      onclick={onClose}
    ></button>

    <div
      role="dialog"
      aria-modal="true"
      aria-label="Talent Network"
      class="relative z-10 flex max-h-[85vh] w-full max-w-lg flex-col overflow-hidden rounded-2xl border border-border bg-background shadow-xl"
      {@attach focusTrap()}
    >
      <div class="flex items-start gap-3 border-b border-border p-5">
        <div class="min-w-0 flex-1">
          <h2 class="text-lg font-semibold leading-tight">Talent Network</h2>
          <p class="mt-0.5 text-sm text-muted-foreground">
            Get discovered without applying anywhere: publish a read-only profile page at a
            private link you can share yourself.
          </p>
        </div>
        <button
          type="button"
          onclick={onClose}
          aria-label="Close"
          class="-mr-1 rounded-full p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
        >
          <X class="size-5" />
        </button>
      </div>

      <div class="flex-1 overflow-y-auto p-5">
        {#if status === 'loading'}
          <p class="text-sm text-muted-foreground">Loading…</p>
        {:else if status === 'error'}
          <!-- The failure itself is already surfaced above via the page's actionError
               banner (onError) — rendering the picker here too would show it defaulted
               to "Off", which may not reflect the account's real, unknown-because-
               unloaded state. -->
          <p class="text-sm text-muted-foreground">Couldn't load this setting.</p>
        {:else}
          <div class="flex flex-col gap-4">
            {#if localError}
              <p class="text-sm text-destructive">{localError}</p>
            {/if}
            <!-- Always rendered, even when visibility is "off" — the public id doesn't
                 change with the setting, only whether the route resolves, so a candidate
                 can find their link without first having to recall their current mode. -->
            <div
              class="flex flex-col gap-2 rounded-lg bg-secondary/50 px-3 py-2.5 sm:flex-row sm:items-center sm:gap-3"
            >
              <span class="min-w-0 flex-1 truncate font-mono text-xs text-foreground"
                >{publicUrl}</span
              >
              <span class="flex shrink-0 gap-2">
                <Button variant="ghost" size="sm" onclick={copyLink}>
                  {copied ? 'Copied' : 'Copy link'}
                </Button>
                <!-- Opens the actual public page in a new tab, so a candidate can see
                     exactly what a recruiter would see. -->
                <Button
                  variant="ghost"
                  size="sm"
                  href={resolve('/talent-network/[publicId]', { publicId })}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  View
                </Button>
              </span>
            </div>

            <div class="grid gap-2 sm:grid-cols-3">
              {#each OPTIONS as opt (opt.id)}
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
                    <span aria-hidden="true">{opt.icon}</span>
                    {opt.label}
                  </span>
                  <span class="text-xs text-muted-foreground">{opt.description}</span>
                </button>
              {/each}
            </div>

            <!-- Visible regardless of the current mode, not just once shared — the
                 trade-off needs to be read before a candidate opts in, not discovered
                 afterward. -->
            <p class="text-xs text-muted-foreground">
              Once you share this link, anyone who has it may have already seen or saved your
              profile — switching back to Off removes the live page but can't undo that.
            </p>
          </div>
        {/if}
      </div>
    </div>
  </div>
{/if}
