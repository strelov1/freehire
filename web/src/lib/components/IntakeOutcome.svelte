<script lang="ts">
  // What became of a link somebody handed in, rendered the same way wherever they
  // handed it in — the contribute form and the search box both mount this. The words
  // are in $lib/intakeOutcome (tested there); this adds the two places the outcome can
  // send you, which only some outcomes have.
  //
  // Presentation is deliberately bare: no border, no padding, no background. The two
  // callers frame an outcome very differently (a panel on a page, a row in a dropdown),
  // and a component that brought its own box would have to be talked out of it twice.
  import { resolve } from '$app/paths';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { t } from '$lib/i18n/t';
  import { intakeOutcomeMessage } from '$lib/intakeOutcome';
  import type { ResolvedLink } from '$lib/types';
  import { messages } from './IntakeOutcome.messages';

  let { resolved }: { resolved: ResolvedLink } = $props();

  // This follows the READER, not the page. The other caller is the header search
  // box, and the header renders on `/my/**` too — so pasting a link there while
  // signed in as a Russian-language account gives a Russian outcome inside an
  // otherwise-English dropdown. That is the right answer (the sentence is
  // addressed to the account, and `locale()` is already `en` for everyone on a
  // public page), but it is not "this can never render Russian off /my/**".
  const s = $derived(t(messages, locale()));
</script>

{intakeOutcomeMessage(resolved, locale())}
{#if resolved.public_slug}
  <a
    href={resolve('/jobs/[slug]', { slug: resolved.public_slug })}
    class="font-medium underline underline-offset-4"
  >
    {s.viewTheJob}
  </a>
{/if}
{#if resolved.company_slug}
  <a
    href={resolve('/companies/[slug]', { slug: resolved.company_slug })}
    class="font-medium underline underline-offset-4"
  >
    {s.viewTheCompany}
  </a>
{/if}
