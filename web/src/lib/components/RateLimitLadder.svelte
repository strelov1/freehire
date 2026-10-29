<script lang="ts">
  import type { RateTier, RateTierLimit } from '#lib/types.js';

  // The public read API's rate ladder, rung by rung, from the figures the rate limiter enforces
  // (`/plans`). Nothing here is a number of its own.
  const { tiers }: { tiers: RateTierLimit[] } = $props();

  const LABEL: Record<RateTier, string> = {
    anonymous: 'No key',
    free: 'Free account',
    pro: 'Pro',
    ultra: 'Ultra',
    api: 'API',
    api_data: 'Data',
  };

  const count = (n: number) => n.toLocaleString('en-US');
</script>

<section class="flex flex-col gap-3" aria-labelledby="rate-limits-title">
  <div class="flex flex-col gap-1">
    <h3 id="rate-limits-title" class="text-lg font-semibold">Rate limits</h3>
    <p class="text-sm text-muted-foreground">
      A key is a full-access API key or a signed-in MCP or CLI connection, and every key of one
      account shares its budget. Without a key, or with an embed key, the budget is per address.
    </p>
  </div>
  <div class="overflow-x-auto">
    <table class="w-full min-w-72 border-collapse text-sm">
      <thead>
        <tr class="border-b border-border text-left text-muted-foreground">
          <th class="py-2 pr-3 font-medium">Tier</th>
          <th class="px-3 py-2 text-right font-medium">Per minute</th>
          <th class="py-2 pl-3 text-right font-medium">Per day</th>
        </tr>
      </thead>
      <tbody>
        {#each tiers as rung (rung.tier)}
          <tr class="border-b border-border/60">
            <td class="py-2 pr-3">{LABEL[rung.tier]}</td>
            <td class="px-3 py-2 text-right font-mono tabular-nums">{count(rung.per_minute)}</td>
            <td class="py-2 pl-3 text-right font-mono tabular-nums">
              {#if rung.per_day > 0}{count(rung.per_day)}{:else}<span class="text-muted-foreground">No cap</span>{/if}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
</section>
