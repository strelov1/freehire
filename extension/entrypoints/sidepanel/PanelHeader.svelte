<script lang="ts">
  import { browser } from 'wxt/browser';
  import { Alert, Badge, Button } from 'freehire-design-system';
  import type { HireUser } from '../../lib/auth';

  // The side panel's header: the brand and build, whether the agent is working, and the
  // signed-in account with its sign-in and sign-out controls.
  let {
    user,
    sending,
    authBusy,
    authError,
    onSignIn,
    onSignOut,
  }: {
    user: HireUser | null;
    sending: boolean;
    authBusy: boolean;
    authError: string;
    onSignIn: () => void;
    onSignOut: () => void;
  } = $props();
</script>

<header>
  <div class="top">
    <span class="brand">
      <img src="/icon/32.png" alt="" class="brand-mark" width="18" height="18" />
      <strong>freehire</strong>
      <!-- The build, in the one place both of us can see it: a side panel keeps
           its script until it is closed, so "did the reload take?" is otherwise
           unanswerable from a screenshot. -->
      <span class="build">v{browser.runtime.getManifest().version}</span>
    </span>
    <Badge variant={sending ? 'brand' : 'outline'}>
      {sending ? 'working…' : user ? 'ready' : 'offline'}
    </Badge>
  </div>
  <div class="auth">
    {#if user}
      <span class="who">Signed in as <b>{user.email}</b></span>
      <button class="link" onclick={onSignOut}>Sign out</button>
    {:else}
      <Button variant="primary" size="sm" onclick={onSignIn} disabled={authBusy}>
        {authBusy ? 'Signing in…' : 'Sign in with freehire'}
      </Button>
    {/if}
  </div>
  {#if authError}
    <Alert variant="destructive">{authError}</Alert>
  {/if}
</header>

<style>
  header {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 10px 12px;
  }

  .top {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .brand-mark {
    border-radius: 4px;
  }

  .auth {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    font-size: 12px;
  }

  .who {
    color: var(--muted-foreground);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .link {
    border: none;
    background: none;
    color: var(--brand-strong);
    cursor: pointer;
    font: inherit;
    font-size: 12px;
    padding: 0;
  }

  .build {
    font-size: 11px;
    color: var(--muted-foreground);
  }
</style>
