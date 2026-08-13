<script lang="ts">
  import { Clock, Mail } from '@lucide/svelte';
  import { companyLogoUrl } from '$lib/logo';
  import { EntityLogo } from '$lib/ui';
  import { groupedStages } from '$lib/stages';
  import { timeAgo } from '$lib/utils';
  import type { BoardItem } from '$lib/board';
  import type { MyJob } from '$lib/types';

  // The same applications the board shows, read as a list. It owns no state and no
  // writes: JobBoard holds both, so the two views cannot drift apart.
  let {
    items,
    onopen,
    onsetstage,
  }: {
    items: BoardItem[];
    onopen: (item: MyJob) => void;
    // (item, stage) rather than (stage): the board's drawer acts on whatever is open,
    // but a list row acts on itself and there is nothing open.
    onsetstage: (item: BoardItem, stage: string) => void;
  } = $props();

  const company = (i: BoardItem) => i.job?.company || i.company_slug;
  const title = (i: BoardItem) => i.job?.title || i.role_title;
</script>

{#if items.length === 0}
  <p class="py-8 text-center text-sm text-muted-foreground">No applications yet.</p>
{:else}
  <ul class="flex flex-col divide-y divide-border rounded-xl border border-border">
    {#each items as item (item.id)}
      <li class="flex items-center gap-3 px-3 py-2.5 transition-colors hover:bg-accent/50">
        <!-- The employer and role open the application; the stage control beside them
             does not, so it is a sibling rather than a child. -->
        <button
          type="button"
          onclick={() => onopen(item)}
          class="flex min-w-0 flex-1 items-center gap-2.5 text-left"
        >
          <EntityLogo name={company(item)} src={companyLogoUrl(company(item)) ?? undefined} shape="square" size="xs" />
          <span class="min-w-0 flex-1">
            <span class="block truncate text-sm font-medium">{title(item)}</span>
            <span class="block truncate text-xs text-muted-foreground">{company(item) || 'Unknown company'}</span>
          </span>
        </button>

        <span class="flex shrink-0 items-center gap-2.5">
          {#if item.silence_state === 'silent'}
            <span
              class="flex items-center gap-0.5 text-xs tabular-nums text-warning-strong"
              title="No reply for {item.days_silent} days"
            >
              <Clock class="size-3 shrink-0" aria-hidden="true" />
              {item.days_silent}d
            </span>
          {/if}
          {#if item.email_count > 0}
            <span
              class="flex items-center gap-0.5 text-xs tabular-nums text-muted-foreground"
              title="{item.email_count} linked email{item.email_count === 1 ? '' : 's'}"
            >
              <Mail class="size-3 shrink-0" aria-hidden="true" />
              {item.email_count}
            </span>
          {/if}
          {#if item.last_activity_at}
            <span class="hidden text-xs text-muted-foreground sm:inline">{timeAgo(item.last_activity_at)}</span>
          {/if}
          <select
            value={item.stage ?? ''}
            onchange={(e) => onsetstage(item, e.currentTarget.value)}
            aria-label="Stage for {title(item)} at {company(item) || 'unknown company'}"
            class="rounded-md border border-input bg-transparent px-2 py-1 text-xs"
          >
            <option value="">No stage</option>
            <!-- Grouped exactly as the drawer's selector is: two stage pickers in one
                 section that organise their options differently is the confusion this
                 change is removing, in miniature. -->
            {#each groupedStages() as g (g.id)}
              <optgroup label={g.label}>
                {#each g.options as s (s.value)}
                  <option value={s.value}>{s.label}</option>
                {/each}
              </optgroup>
            {/each}
          </select>
        </span>
      </li>
    {/each}
  </ul>
{/if}
