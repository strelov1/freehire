<script lang="ts">
  import { Bell } from '@lucide/svelte';
  import { ApiError } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { canonicalQuery } from '$lib/filters';
  import { savedSearches } from '$lib/savedSearches.svelte';
  import { notifications } from '$lib/notifications.svelte';
  import { Button } from '$lib/ui';

  // "Subscribe to updates" on a company page: follow this company's new postings as
  // Telegram digests, built entirely from the saved-search + filter-subscription
  // primitives. A follow is a saved search whose query is exactly
  // `company_slug=<slug>` (named after the company) plus a Telegram subscription on
  // it. The Telegram connect flow mirrors SavedSearches.svelte.
  let { slug, companyName }: { slug: string; companyName: string } = $props();

  let busy = $state(false);
  let error = $state<string | null>(null);
  // Set after opening the connect deep link, so we can offer an "I've connected"
  // recheck that completes the follow the user intended.
  let connecting = $state(false);

  // The canonical query standing for "only this company", produced the same way the
  // filters panel and cmd/notify build it — so a matching saved search is recognised.
  const companyQuery = $derived(
    canonicalQuery(new URLSearchParams({ company_slug: slug }).toString()),
  );

  const telegram = $derived(notifications.telegram);
  // The saved search that is exactly this company filter, if any (reused on follow).
  const savedSearch = $derived(
    savedSearches.items.find((s) => canonicalQuery(s.query) === companyQuery),
  );
  const sub = $derived(savedSearch ? notifications.forSavedSearch(savedSearch.id) : undefined);
  const subscribed = $derived(!!sub);

  // Load the stores once the session is confirmed (boot-time /me may still be in
  // flight). SSR-safe: ensureLoaded is a browser-only no-op. On sign-out, drop the
  // per-user caches so the next user on this tab loads their own state.
  $effect(() => {
    if (isAuthenticated()) {
      void savedSearches.ensureLoaded();
      void notifications.ensureLoaded();
    } else {
      savedSearches.reset();
      notifications.reset();
    }
  });

  async function toggle() {
    if (busy) return;
    if (!isAuthenticated()) {
      openAuthDialog();
      return;
    }
    busy = true;
    error = null;
    try {
      if (subscribed) {
        await unfollow();
      } else {
        await follow();
      }
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not update. Please try again.';
    } finally {
      busy = false;
    }
  }

  async function follow() {
    // Telegram must be linked before digests can be delivered; walk the connect
    // flow first and let the recheck finish the follow.
    if (!telegram.linked) {
      await connectTelegram();
      return;
    }
    // Reuse a matching saved search (no duplicates, no unique-name clash); otherwise
    // create one named after the company — the name doubles as the digest title.
    const set = savedSearch ?? (await savedSearches.create(companyName, companyQuery));
    await notifications.subscribe(set.id);
  }

  async function unfollow() {
    if (!sub || !savedSearch) return;
    await notifications.unsubscribe(sub.id);
    // Clean toggle: also drop the saved search, but only when it is the one we
    // generated (name === company name), so a user's own filter for this company —
    // named differently — is preserved.
    if (savedSearch.name === companyName) {
      await savedSearches.remove(savedSearch.id);
    }
  }

  // Open the one-time deep link in a new tab so the user can tap Start in Telegram.
  async function connectTelegram() {
    const url = await notifications.link();
    window.open(url, '_blank', 'noopener');
    connecting = true;
  }

  // After the user reports they tapped Start, re-read the link status and, if now
  // linked, complete the follow they were in the middle of.
  async function recheckLink() {
    if (busy) return;
    busy = true;
    error = null;
    try {
      await notifications.refreshTelegram();
      if (notifications.telegram.linked) {
        connecting = false;
        await follow();
      }
    } catch {
      error = 'Could not check the connection. Please try again.';
    } finally {
      busy = false;
    }
  }
</script>

<!-- Hidden only for a signed-in user whose Telegram feature is off server-side
     (nothing to deliver). Signed-out users still see it and are routed to sign in. -->
{#if !isAuthenticated() || telegram.enabled}
  <div class="flex flex-col items-end gap-1">
    {#if connecting && !telegram.linked}
      <Button variant="secondary" size="sm" onclick={recheckLink} disabled={busy}>
        {busy ? 'Checking…' : 'I’ve connected'}
      </Button>
      <p class="text-xs text-muted-foreground">Opened Telegram — tap “Start”, then confirm.</p>
    {:else}
      <Button
        variant={subscribed ? 'secondary' : 'primary'}
        size="sm"
        onclick={toggle}
        disabled={busy}
        aria-pressed={subscribed}
        aria-label={subscribed ? 'Subscribed' : 'Subscribe to updates'}
      >
        <Bell class="size-4" aria-hidden="true" />
        <!-- Icon-only on mobile to keep the company header compact; label from sm up. -->
        <span class="hidden sm:inline">{subscribed ? 'Subscribed' : 'Subscribe to updates'}</span>
      </Button>
    {/if}
    {#if error}
      <p class="text-xs text-destructive">{error}</p>
    {/if}
  </div>
{/if}
