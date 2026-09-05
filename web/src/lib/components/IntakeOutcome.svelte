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
  import { intakeOutcomeMessage } from '$lib/intakeOutcome';
  import type { ResolvedLink } from '$lib/types';

  let { resolved }: { resolved: ResolvedLink } = $props();
</script>

{intakeOutcomeMessage(resolved)}
{#if resolved.public_slug}
  <a
    href={resolve('/jobs/[slug]', { slug: resolved.public_slug })}
    class="font-medium underline underline-offset-4"
  >
    View the job →
  </a>
{/if}
{#if resolved.company_slug}
  <a
    href={resolve('/companies/[slug]', { slug: resolved.company_slug })}
    class="font-medium underline underline-offset-4"
  >
    View the company →
  </a>
{/if}
