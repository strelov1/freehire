<script lang="ts">
  import { resolve } from '$app/paths';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';
  import { badgeVariants, cn } from '#lib/ui/index.js';
  import { messages } from './PlanBadge.messages';

  // The caller's current plan as a link: Free leads to the benefits a purchase adds
  // (/pricing), a paid tier to the page that shows its allowances (/my/plan).
  //
  // `tier` is the resolved candidate tier from the session. A signed-out visitor or an
  // identity whose tier did not arrive has none, and a missing tier is not "free" —
  // so nothing renders rather than a label that may be wrong.
  let { tier, class: className }: { tier?: 'free' | 'pro' | 'ultra' | null; class?: string } = $props();

  const s = $derived(t(messages, locale()));
  const known = $derived(tier === 'free' || tier === 'pro' || tier === 'ultra' ? tier : null);
</script>

{#if known}
  <a
    href={resolve(known === 'free' ? '/pricing' : '/my/plan')}
    aria-label={known === 'free' ? s.freeLabel : format(s.paidLabel, { tier: s[known] })}
    class={cn(
      badgeVariants({ variant: known === 'free' ? 'outline' : 'brand' }),
      'shrink-0 font-semibold transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
      className,
    )}
  >
    {s[known]}
  </a>
{/if}
