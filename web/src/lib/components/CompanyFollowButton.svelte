<script lang="ts">
  import { Bell } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { ApiError } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { promptSignIn } from '$lib/signin';
  import { canonicalQuery } from '$lib/filters';
  import { savedSearches } from '$lib/savedSearches.svelte';
  import { notifications } from '$lib/notifications.svelte';
  import { Button } from '$lib/ui';

  // "Subscribe to updates" on a company page: follow this company's new postings as
  // Telegram digests, built entirely from the saved-search + filter-subscription
  // primitives. A follow is a saved search whose query is exactly
  // `company_slug=<slug>` (named after the company) plus a Telegram subscription on
  // it. Connecting the bot itself lives on Integrations (/my/integrations) — while
  // it isn't linked, this renders as a link there instead of a toggle.
  let { slug, companyName }: { slug: string; companyName: string } = $props();

  let busy = $state(false);
  let error = $state<string | null>(null);

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
      promptSignIn();
      return;
    }
    if (!telegram.linked) return; // the button renders as a link to Integrations instead
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
</script>

<!-- Hidden only for a signed-in user whose Telegram feature is off server-side
     (nothing to deliver). Signed-out users still see it and are routed to sign in. -->
{#if !isAuthenticated() || telegram.enabled}
  <div class="flex flex-col items-end gap-1">
    {#if isAuthenticated() && !telegram.linked}
      <Button variant="outline" size="sm" href={resolve('/my/integrations')}>
        <Bell class="size-4" aria-hidden="true" />
        <span class="hidden sm:inline">Connect Telegram to follow</span>
      </Button>
    {:else}
      <Button
        variant={subscribed ? 'secondary' : 'outline'}
        class={subscribed
          ? undefined
          : 'border-brand-strong bg-transparent text-brand-strong hover:bg-brand-muted'}
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
