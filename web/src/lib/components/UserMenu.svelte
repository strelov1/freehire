<script lang="ts">
  import { currentUser, logout as doLogout } from '$lib/auth.svelte';

  // The dropdown closes on outside click (window listener checks containment),
  // on Escape, and after the menu action itself.
  let open = $state(false);
  let root = $state<HTMLElement | null>(null);

  const email = $derived(currentUser()?.email ?? '');
  const initial = $derived(email.charAt(0).toUpperCase());
  const isModerator = $derived(currentUser()?.role === 'moderator');

  function onWindowClick(e: MouseEvent) {
    if (open && root && !root.contains(e.target as Node)) open = false;
  }

  function logout() {
    open = false;
    void doLogout();
  }
</script>

<svelte:window
  onclick={onWindowClick}
  onkeydown={(e) => e.key === 'Escape' && (open = false)}
/>

<div class="relative" bind:this={root}>
  <button
    type="button"
    aria-label="Account menu"
    aria-haspopup="menu"
    aria-expanded={open}
    onclick={() => (open = !open)}
    class="flex size-8 items-center justify-center rounded-full bg-secondary text-sm font-medium text-secondary-foreground transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
  >
    {initial}
  </button>

  {#if open}
    <div
      role="menu"
      class="absolute right-0 top-full z-50 mt-2 w-56 rounded-md border border-border bg-background py-1 shadow-lg"
    >
      <p class="truncate px-3 py-2 text-sm text-muted-foreground" title={email}>{email}</p>
      <div class="my-1 h-px bg-border"></div>
      <a
        href="/my/jobs"
        role="menuitem"
        onclick={() => (open = false)}
        class="block px-3 py-2 text-sm transition-colors hover:bg-accent hover:text-accent-foreground"
      >
        My jobs
      </a>
      <a
        href="/my/notifications"
        role="menuitem"
        onclick={() => (open = false)}
        class="block px-3 py-2 text-sm transition-colors hover:bg-accent hover:text-accent-foreground"
      >
        Notifications
      </a>
      <a
        href="/my/api-keys"
        role="menuitem"
        onclick={() => (open = false)}
        class="block px-3 py-2 text-sm transition-colors hover:bg-accent hover:text-accent-foreground"
      >
        API keys
      </a>
      <div class="my-1 h-px bg-border"></div>
      <a
        href="/submit"
        role="menuitem"
        onclick={() => (open = false)}
        class="block px-3 py-2 text-sm transition-colors hover:bg-accent hover:text-accent-foreground"
      >
        Submit a job
      </a>
      <a
        href="/my/submissions"
        role="menuitem"
        onclick={() => (open = false)}
        class="block px-3 py-2 text-sm transition-colors hover:bg-accent hover:text-accent-foreground"
      >
        My submissions
      </a>
      {#if isModerator}
        <a
          href="/moderation"
          role="menuitem"
          onclick={() => (open = false)}
          class="block px-3 py-2 text-sm transition-colors hover:bg-accent hover:text-accent-foreground"
        >
          Moderation
        </a>
      {/if}
      <div class="my-1 h-px bg-border"></div>
      <button
        type="button"
        role="menuitem"
        onclick={logout}
        class="w-full px-3 py-2 text-left text-sm transition-colors hover:bg-accent hover:text-accent-foreground"
      >
        Log out
      </button>
    </div>
  {/if}
</div>
