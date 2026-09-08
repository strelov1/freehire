<script lang="ts">
  // The email preference centre, reached from the footer of every non-essential
  // mail. Public on purpose: the signed token in the link stands in for a session,
  // because the whole feature is that somebody holding one of our mails can turn it
  // off without an account, a password, or a support ticket.
  //
  // It shows the address the mail went to, three switches, and the names of the
  // searches those switches govern. Nothing else about the account reaches this
  // page — the token could have arrived by a forwarded email.
  import { onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import ToggleSwitch from '$lib/components/ToggleSwitch.svelte';
  import { cn } from '$lib/ui';
  import { api, type EmailPrefs } from '$lib/api';

  // Named Phase, and the variable `phase`, because a variable called `state`
  // shadows the $state rune in a Svelte 5 component and the resulting error names
  // the rune rather than the shadowing.
  type Phase = 'loading' | 'ready' | 'invalid';

  let phase = $state<Phase>('loading');
  let prefs = $state<EmailPrefs | null>(null);
  let saving = $state(false);
  let saveFailed = $state(false);
  // The token is kept in a variable rather than re-read from the URL, because the
  // URL loses it a moment after the first read — see stripTokenFromTheAddressBar.
  let token = '';

  onMount(async () => {
    token = page.url.searchParams.get('t') ?? '';
    if (!token) {
      phase = 'invalid';
      return;
    }
    try {
      prefs = await api.getEmailPrefs(token);
      phase = 'ready';
    } catch {
      phase = 'invalid';
      return;
    }
    stripTokenFromTheAddressBar();
  });

  /** Take `?t=` out of the address bar once it has been read.
   *
   *  The token never expires, so every place a URL comes to rest is a place it keeps
   *  working: browser history, a screenshot, a pasted link in a chat. It has to be in
   *  the URL to arrive at all — a link in an email has nowhere else to carry it — but
   *  it does not have to stay there.
   *
   *  The browser's own history.replaceState, NOT SvelteKit's. SvelteKit's throws when
   *  the router is not ready yet, and that failure only appears in a production build,
   *  where the message is also unreadable. This is not shallow routing — nothing here
   *  re-reads page.url, because the token is already held in a variable — so the
   *  router has no part to play and cannot be a reason the page dies.
   *
   *  Runs after the first successful read, so a failed load leaves the URL intact and
   *  the page can simply be reloaded. */
  function stripTokenFromTheAddressBar() {
    const clean = new URL(window.location.href);
    clean.searchParams.delete('t');
    window.history.replaceState(window.history.state, '', clean);
  }

  async function save(update: Parameters<typeof api.saveEmailPrefs>[1]) {
    if (!prefs) return;
    saving = true;
    saveFailed = false;
    try {
      prefs = await api.saveEmailPrefs(token, update);
    } catch {
      saveFailed = true;
    } finally {
      saving = false;
    }
  }

  const toggleAlerts = () => save({ alerts: !prefs?.alerts_enabled });
  const toggleActivity = () => save({ activity: !prefs?.activity_enabled });
  const toggleNews = () => save({ news: !prefs?.news_enabled });

  /** Turn one saved-search digest off. There is no matching "on": the endpoint can
   *  only deactivate, so a link that leaked cannot sign anybody up for anything. */
  const turnSearchOff = (id: number) => save({ deactivateSearches: [id] });

  const unsubscribeFromEverything = () =>
    save({ alerts: false, activity: false, news: false });

  const allOff = $derived(
    !!prefs && !prefs.alerts_enabled && !prefs.activity_enabled && !prefs.news_enabled,
  );
</script>

<svelte:head>
  <title>Email settings — freehire</title>
  <meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-xl px-4 py-12">
  {#if phase === 'loading'}
    <p class="text-sm text-muted-foreground">Loading your email settings…</p>
  {:else if phase === 'invalid'}
    <!-- One message for every refusal. Telling a forged token apart from a deleted
         account would let an unauthenticated visitor learn which accounts exist. -->
    <h1 class="text-xl font-semibold">This link is no longer valid</h1>
    <p class="mt-3 text-sm text-muted-foreground">
      Open the most recent email you had from us and use the link in its footer. If you
      have an account, you can also change everything from
      <a class="underline" href={resolve('/my/notifications')}>your notification settings</a>.
    </p>
  {:else if prefs}
    <h1 class="text-xl font-semibold">Email settings</h1>
    <p class="mt-1 text-sm text-muted-foreground">{prefs.email}</p>

    {#if saveFailed}
      <p role="alert" class="mt-4 text-sm text-destructive">
        That didn’t save. Try again — nothing was changed.
      </p>
    {/if}

    <div class="mt-8 space-y-6">
      {@render group(
        'Job alerts',
        'New matches for the searches you saved.',
        prefs.alerts_enabled,
        toggleAlerts,
      )}

      {#if prefs.searches.length > 0}
        <ul class="ml-4 space-y-3 border-l border-border pl-4">
          {#each prefs.searches as search (search.id)}
            <li class="flex items-center justify-between gap-4">
              <span
                class={cn('text-sm', search.active ? '' : 'text-muted-foreground line-through')}
              >
                {search.name}
              </span>
              {#if search.active}
                <button
                  type="button"
                  class="text-sm underline disabled:opacity-50"
                  disabled={saving}
                  onclick={() => turnSearchOff(search.id)}
                >
                  Turn off
                </button>
              {:else}
                <span class="text-sm text-muted-foreground">Off</span>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}

      {@render group(
        'Notifications',
        'Reminders about jobs you saved, follow-ups on your applications, and answers to anything you reported.',
        prefs.activity_enabled,
        toggleActivity,
      )}

      {@render group(
        'News and product updates',
        'Occasional letters about what we’ve built.',
        prefs.news_enabled,
        toggleNews,
      )}
    </div>

    <div class="mt-10 border-t border-border pt-6">
      <button
        type="button"
        class="text-sm underline disabled:opacity-50"
        disabled={saving || allOff}
        onclick={unsubscribeFromEverything}
      >
        {allOff ? 'You’re unsubscribed from everything' : 'Unsubscribe from everything'}
      </button>
      <p class="mt-3 text-xs text-muted-foreground">
        Security emails — confirming your address, resetting your password — are always
        sent. There is no setting that stops them.
      </p>
    </div>
  {/if}
</div>

{#snippet group(title: string, description: string, on: boolean, toggle: () => void)}
  <div class="flex items-start justify-between gap-4">
    <div>
      <h2 class="text-sm font-semibold leading-tight">{title}</h2>
      <p class="mt-1 text-sm text-muted-foreground">{description}</p>
    </div>
    <ToggleSwitch {on} label={title} disabled={saving} onToggle={toggle} />
  </div>
{/snippet}
