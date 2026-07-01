<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { replaceState } from '$app/navigation';
  import { authDialog, openAuthDialog, closeAuthDialog } from '$lib/auth-dialog.svelte';
  import { cn } from '$lib/utils';
  import AuthDialog from './AuthDialog.svelte';
  import HeaderSearch from './HeaderSearch.svelte';
  import HeaderListSearch from './HeaderListSearch.svelte';
  import HeaderMenu from './HeaderMenu.svelte';

  // The header is three slots — logo | search | menu — identical on every
  // viewport. Nav links, the account items, the theme toggle, and the auth
  // action all live in HeaderMenu.
  //
  // The middle slot is one text field that adapts to context: on the list pages
  // (/jobs, /companies) it IS that page's filter (HeaderListSearch drives the
  // list, so there's no duplicate box); everywhere else it's the global launcher
  // with the instant dropdown (HeaderSearch).
  const listKind = $derived(
    page.url.pathname === '/jobs' ? 'jobs' : page.url.pathname === '/companies' ? 'companies' : null,
  );

  // Jobs/Companies are also surfaced as inline links here — left-aligned beside
  // the logo, shown only from sm up (mobile keeps them in HeaderMenu, which is
  // unchanged). The active section is emphasised, matching HeaderMenu's links.
  const path = $derived(page.url.pathname);
  const isActive = (href: string) => path === href || path.startsWith(`${href}/`);
  const navLinkClass = (href: string) =>
    cn(
      'rounded-md px-2 py-1.5 text-sm transition-colors hover:bg-accent hover:text-accent-foreground',
      isActive(href) ? 'font-medium text-foreground' : 'text-muted-foreground',
    );

  // The auth dialog lives at the layout level but its open state is a shared
  // singleton (see auth-dialog.svelte), so deep components — like a job's Save
  // button — can prompt sign-in through the same dialog this header renders.

  // Surface auth prompts carried in the URL once on the client, then clean it.
  // ?auth_error: a failed OAuth callback. ?auth=required: a guarded page (e.g.
  // /my/jobs) bounced a signed-out visitor here to sign in. In onMount so it
  // never runs during SSR.
  onMount(() => {
    const params = page.url.searchParams;
    if (params.has('auth_error')) {
      // A real failure: seed the dialog's error banner.
      openAuthDialog('login', 'Sign-in failed. Please try again.');
    } else if (params.get('auth') === 'required') {
      // Just a sign-in gate, not an error — open the dialog with no error banner.
      openAuthDialog('login');
    } else {
      return;
    }
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- shallow same-page URL clean-up to the current pathname; nothing to resolve
    replaceState(page.url.pathname, {});
  });
</script>

<header class="border-b border-border">
  <div class="mx-auto flex h-14 max-w-6xl items-center gap-3 px-4 sm:gap-4">
    <a
      href={resolve('/')}
      class="flex shrink-0 items-center gap-2 text-sm font-semibold tracking-tight"
    >
      <!-- The mark inherits text colour via currentColor, so it tracks the theme
           (light/dark) exactly like the wordmark beside it. aria-hidden: the
           "FreeHire" text already names the link. -->
      <svg
        viewBox="0 0 512 512"
        class="size-5 shrink-0"
        fill="currentColor"
        aria-hidden="true"
        xmlns="http://www.w3.org/2000/svg"
      >
        <path
          fill-rule="evenodd"
          clip-rule="evenodd"
          d="M256 56C366.457 56 456 145.543 456 256C456 366.457 366.457 456 256 456C145.543 456 56 366.457 56 256C56 145.543 145.543 56 256 56ZM256 166L346 256L256 346L166 256L256 166Z"
        />
      </svg>
      <span class="hidden sm:inline">FreeHire</span>
    </a>

    <nav class="hidden shrink-0 items-center gap-1 sm:flex">
      <a href={resolve('/jobs')} class={navLinkClass('/jobs')}>Jobs</a>
      <a href={resolve('/companies')} class={navLinkClass('/companies')}>Companies</a>
    </nav>

    {#if listKind === 'jobs'}
      <HeaderListSearch placeholder="Search jobs…" />
    {:else if listKind === 'companies'}
      <HeaderListSearch placeholder="Search companies…" />
    {:else}
      <HeaderSearch />
    {/if}

    <HeaderMenu />
  </div>
</header>

{#if authDialog.open}
  <AuthDialog bind:mode={authDialog.mode} initialError={authDialog.error} onClose={closeAuthDialog} />
{/if}
