<script lang="ts">
  import { credentialBadges } from '#lib/credentials.js';
  import { cn } from '#lib/ui/index.js';

  // Renders an employer's register-backed credentials — currently the UK and Dutch
  // visa-sponsor licences.
  //
  // Deliberately quiet: a licence is a fact about the employer, not a feature of
  // the role, so it reads as metadata alongside the other chips rather than as a
  // selling point. The disclaimer rides in the tooltip on every badge, because the
  // failure mode is a candidate reading "licensed sponsor" as "they will sponsor
  // me" and spending an application on it.
  let { collections, class: className }: { collections?: string[] | null; class?: string } =
    $props();

  const badges = $derived(credentialBadges(collections));
</script>

{#each badges as badge (badge.slug)}
  <span
    title={badge.tooltip}
    class={cn(
      'inline-flex items-center gap-1.5 rounded-md border border-border px-2 py-0.5 text-xs font-medium text-muted-foreground',
      className,
    )}
  >
    {badge.label}
    <span aria-hidden="true" class="opacity-70">{badge.issuer}</span>
    <!-- The disclaimer cannot live in `title` alone. There is no hover on a phone,
         and a large share of job-search traffic is on one, so the qualifier would be
         unreachable for exactly the readers most likely to act on the badge; screen
         readers announce `title` inconsistently too. This carries the full sentence
         into the accessible name and leaves the visual chip unchanged. -->
    <span class="sr-only">— {badge.tooltip}</span>
  </span>
{/each}
