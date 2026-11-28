<script lang="ts">
  import { resolve } from '$app/paths';
  import { BadgeCheck } from '@lucide/svelte';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { ensureEmployerAccountLoaded, hasEmployerAccount } from '#lib/employerAccount.svelte.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { messages } from './ClaimCompanyLink.messages.js';

  // The way in to claiming a company, shown as the last line of the company header, on the company's main page only. It lives
  // under /my, so a signed-out reader is sent through sign-in and brought back to the claim
  // form with the company already named. Hidden once somebody holds the company (`claimed`,
  // from the page's own response) or when the reader already acts for a company.
  let { slug, claimed }: { slug: string; claimed: boolean } = $props();

  $effect(() => {
    if (isAuthenticated()) void ensureEmployerAccountLoaded();
  });
</script>

{#if !claimed && !hasEmployerAccount()}
  <!-- eslint-disable svelte/no-navigation-without-resolve -- resolved route plus a query string -->
  <a
    href={`${resolve('/my/employer')}?company=${encodeURIComponent(slug)}`}
    class="-mb-2 mt-1 inline-flex w-fit items-center gap-1 text-xs text-muted-foreground hover:text-foreground hover:underline"
  >
    <BadgeCheck class="size-3.5" aria-hidden="true" />
    {t(messages, locale()).label}
  </a>
  <!-- eslint-enable svelte/no-navigation-without-resolve -->
{/if}
