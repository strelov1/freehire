<script lang="ts">
  import { Check, Minus } from '@lucide/svelte';
  import { ghostBadge, ghostChecklist } from '$lib/ghost';
  import type { Ghost } from '$lib/generated/contracts';

  // The full justification behind the chip: every criterion the classifier weighs,
  // the fired ones with their facts, the rest explicitly marked as having no data.
  //
  // Showing the unfired rows is the point, not clutter. They tell the reader WHY the
  // verdict is not stronger — a posting flagged on two structural criteria with no
  // outcome data is a very different thing from one several people reported — and
  // that is the difference between informing somebody and accusing an employer at
  // them.
  let { ghost }: { ghost?: Ghost | null } = $props();

  const badge = $derived(ghostBadge(ghost));
  const rows = $derived(ghost && badge ? ghostChecklist(ghost) : []);
</script>

{#if badge && rows.length}
  <section class="rounded-lg border border-border p-4">
    <div class="mb-3 flex items-baseline justify-between gap-4">
      <h3 class="text-sm font-semibold tracking-tight">Signs this posting may be inactive</h3>
      <span class="text-xs tabular-nums text-muted-foreground">{badge.scale}</span>
    </div>

    <ul class="flex flex-col gap-2">
      {#each rows as row (row.code)}
        <li class="flex items-start gap-2.5 text-sm">
          {#if row.fired}
            <Check class="mt-0.5 size-4 shrink-0 text-amber-600 dark:text-amber-400" />
          {:else}
            <Minus class="mt-0.5 size-4 shrink-0 text-muted-foreground/50" />
          {/if}
          <span class="flex min-w-0 flex-col">
            <span class={row.fired ? '' : 'text-muted-foreground'}>{row.label}</span>
            <span class="text-xs text-muted-foreground">{row.detail}</span>
          </span>
        </li>
      {/each}
    </ul>

    <p class="mt-3 text-xs text-muted-foreground">
      These are observations about the posting, not a claim about the employer.
    </p>
  </section>
{/if}
