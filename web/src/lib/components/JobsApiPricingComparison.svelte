<script lang="ts">
  import { Check, Minus } from '@lucide/svelte';

  // The comparison a buyer is already making. The other two columns describe kinds of
  // product, not a named competitor, and say "often" where the market varies — a claim
  // about a rival we cannot point to is one we do not make.
  type Cell = { mark?: 'yes' | 'no'; text: string };
  const comparison: { row: string; ours: Cell; metered: Cell; diy: Cell }[] = [
    {
      row: 'What you pay for',
      ours: { mark: 'yes', text: 'A flat monthly plan' },
      metered: { text: 'Every record you read, plus overage' },
      diy: { text: 'Engineers, proxies and servers' },
    },
    {
      row: 'Try it before you pay',
      ours: { mark: 'yes', text: 'Without a key or a card' },
      metered: { text: 'A trial, after sign-up' },
      diy: { mark: 'no', text: 'Weeks of building first' },
    },
    {
      row: 'Full description in every result',
      ours: { mark: 'yes', text: 'Up to 100 postings a page' },
      metered: { text: 'Often a separate call' },
      diy: { text: 'Whatever you manage to parse' },
    },
    {
      row: 'De-duplicated across sources',
      ours: { mark: 'yes', text: 'Reposts collapsed' },
      metered: { text: 'Varies' },
      diy: { mark: 'no', text: 'You build it' },
    },
    {
      row: 'Company profile on each posting',
      ours: { mark: 'yes', text: 'Included' },
      metered: { text: 'Often a paid add-on' },
      diy: { mark: 'no', text: 'You build it' },
    },
    {
      row: 'Licence to show it in your product',
      ours: { mark: 'yes', text: 'Plain, published in our terms' },
      metered: { text: 'Per contract' },
      diy: { mark: 'no', text: 'Your own legal risk' },
    },
    {
      row: 'MCP server for AI agents',
      ours: { mark: 'yes', text: 'Included' },
      metered: { text: 'Rare' },
      diy: { mark: 'no', text: 'You build it' },
    },
    {
      row: 'Sales call to start',
      ours: { mark: 'yes', text: 'Never' },
      metered: { text: 'Often, for volume' },
      diy: { text: 'Not needed' },
    },
  ];
</script>

<!-- The /jobs-api landing's pricing comparison: freehire beside a record-metered API and a
     crawler of your own. -->
{#snippet cell(c: Cell, strong = false)}
  <span class="flex items-start gap-2 {strong ? 'text-foreground' : 'text-muted-foreground'}">
    {#if c.mark === 'yes'}
      <Check class="mt-0.5 size-4 shrink-0 text-primary" aria-hidden="true" />
    {:else if c.mark === 'no'}
      <Minus class="mt-0.5 size-4 shrink-0" aria-hidden="true" />
    {/if}
    {c.text}
  </span>
{/snippet}

<section class="border-t border-border py-14 sm:py-16">
  <h2 class="max-w-2xl text-2xl font-semibold tracking-tight sm:text-3xl">One flat price, however much you read.</h2>
  <p class="mt-4 max-w-2xl leading-relaxed text-muted-foreground">
    The usual job-data pricing meters every record: around $100 a month buys tens of thousands of postings, and
    mirroring a catalogue of millions runs to thousands. Here the bill does not move with how much you read.
  </p>
  <div class="mt-10 overflow-x-auto rounded-xl border border-border">
    <table class="w-full min-w-2xl text-left text-sm">
      <thead>
        <tr class="border-b border-border">
          <th scope="col" class="w-1/4 p-4 font-medium text-muted-foreground"><span class="sr-only">Feature</span></th>
          <th scope="col" class="w-1/4 bg-primary/5 p-4 font-semibold">freehire</th>
          <th scope="col" class="w-1/4 p-4 font-medium text-muted-foreground">Record-metered API</th>
          <th scope="col" class="w-1/4 p-4 font-medium text-muted-foreground">Your own crawler</th>
        </tr>
      </thead>
      <tbody>
        {#each comparison as r (r.row)}
          <tr class="border-b border-border last:border-b-0">
            <th scope="row" class="p-4 font-medium">{r.row}</th>
            <td class="bg-primary/5 p-4">{@render cell(r.ours, true)}</td>
            <td class="p-4">{@render cell(r.metered)}</td>
            <td class="p-4">{@render cell(r.diy)}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
</section>
