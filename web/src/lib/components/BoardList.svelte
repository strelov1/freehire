<script lang="ts">
  import BoardPostingStatus from './BoardPostingStatus.svelte';
  import BoardRemoveButton from './BoardRemoveButton.svelte';
  import BoardPreview from './BoardPreview.svelte';
  import { tick } from 'svelte';
  import { Clock, Mail } from '@lucide/svelte';
  import { companyLogoUrl } from '#lib/logo.js';
  import { EntityLogo, LoadMore } from '#lib/ui/index.js';
  import { groupedStages } from '#lib/stages.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { timeAgo } from '#lib/utils.js';
  import type { BoardItem } from '#lib/board.js';
  import type { MyJob } from '#lib/types.js';
  import InfiniteScroll from './InfiniteScroll.svelte';
  import { messages } from './BoardList.messages';
  import { format, plural, t } from '#lib/i18n/t.js';

  // The same applications the board shows, read as a list. It owns no state and no
  // writes: JobBoard holds both, so the two views cannot drift apart.
  let {
    items,
    onopen,
    onsetstage,
    onremove,
    removing = [],
  }: {
    items: BoardItem[];
    onremove?: (item: MyJob) => void;
    removing?: string[];
    onopen: (item: MyJob) => void;
    // (item, stage) rather than (stage): the board's drawer acts on whatever is open,
    // but a list row acts on itself and there is nothing open.
    onsetstage: (item: BoardItem, stage: string) => void;
  } = $props();

  const company = (i: BoardItem) => i.job?.company || i.company_slug;
  const title = (i: BoardItem) => i.job?.title || i.role_title;
  const s = $derived(t(messages, locale()));

  // Rows are drawn a page at a time as the list scrolls: each carries a stage select of
  // every stage, so 500 applications were ~10k nodes built on load. Nothing is fetched —
  // the rows are already here — so there is no failure to retry.
  const PAGE = 50;
  let limit = $state(PAGE);
  const visible = $derived(items.slice(0, limit));
  const showMore = () => (limit += PAGE);
  let list: HTMLUListElement | undefined = $state();
  let fallback: HTMLDivElement | undefined = $state();

  // The button goes away with the last page, and focus would fall to <body> — back to the
  // top for a keyboard user. It moves to the first row the press drew instead.
  async function showMoreFromKeyboard() {
    const first = limit;
    showMore();
    await tick();
    list?.children[first]?.querySelector('button')?.focus();
  }

  // Tabbing onto the fallback scrolls it into view, and the sentinel just above it with
  // it — so the observer can draw a page while the button holds focus. Only the last page
  // takes the button away; then focus moves, or it falls to <body> all the same. Before
  // that it stays on the button, which the reader has not pressed.
  const showMoreFromSentinel = () =>
    fallback?.contains(document.activeElement) && limit + PAGE >= items.length
      ? showMoreFromKeyboard()
      : showMore();
</script>

{#if items.length === 0}
  <p class="py-8 text-center text-sm text-muted-foreground">{s.empty}</p>
{:else}
  <ul bind:this={list} class="flex flex-col divide-y divide-border rounded-xl border border-border">
    {#each visible as item (item.id)}
      <li class="flex flex-wrap items-center gap-x-3 gap-y-2 px-3 py-3 transition-colors hover:bg-accent/50 sm:flex-nowrap sm:gap-4">
        <!-- The employer and role open the application; the stage control beside them
             does not, so it is a sibling rather than a child. -->
        <BoardPreview {item} class="min-w-0 flex-1">
        <button
          type="button"
          onclick={() => onopen(item)}
          class="flex min-h-11 w-full min-w-0 items-center gap-2.5 rounded text-left focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
        >
          <EntityLogo name={company(item)} src={companyLogoUrl(company(item)) ?? undefined} shape="square" size="xs" />
          <span class="min-w-0 flex-1">
            <span class="block line-clamp-2 text-sm font-medium sm:truncate">{title(item)}</span>
            <span class="block truncate text-xs text-muted-foreground">{company(item) || s.unknownCompany}</span>
            <BoardPostingStatus job={item.job} />
          </span>
        </button>
        </BoardPreview>

        {#if onremove}
          <div class="sm:order-last">
            <BoardRemoveButton {item} {onremove} disabled={removing.includes(item.id)} />
          </div>
        {/if}
        <div class="flex basis-full flex-wrap items-center gap-2 pl-7 sm:basis-auto sm:flex-nowrap sm:gap-3 sm:pl-0">
          <select
            value={item.stage ?? ''}
            onchange={(e) => onsetstage(item, e.currentTarget.value)}
            aria-label={format(s.stageAria, { title: title(item), company: company(item) || s.unknownCompanyLower })}
            class="h-8 w-32 max-w-full shrink-0 rounded-md border border-input bg-background px-2 py-1 text-xs sm:text-sm"
          >
            <option value="">{s.noStage}</option>
            <!-- Grouped exactly as the drawer's selector is: two stage pickers in one
                 section that organise their options differently is the confusion this
                 change is removing, in miniature. -->
            {#each groupedStages(locale()) as g (g.id)}
              <optgroup label={g.label}>
                {#each g.options as s (s.value)}
                  <option value={s.value}>{s.label}</option>
                {/each}
              </optgroup>
            {/each}
          </select>

          {#if item.silence_state === 'silent'}
            <span
              class="flex items-center gap-0.5 text-xs tabular-nums text-warning-strong"
              title={format(plural(locale(), item.days_silent ?? 0, s.noReplyTooltip), { days: String(item.days_silent ?? 0) })}
            >
              <Clock class="size-3 shrink-0" aria-hidden="true" />
              {item.days_silent}d
            </span>
          {/if}
          {#if item.email_count > 0}
            <span
              class="flex items-center gap-0.5 text-xs tabular-nums text-muted-foreground"
              title={format(plural(locale(), item.email_count, s.linkedEmails), { count: String(item.email_count) })}
            >
              <Mail class="size-3 shrink-0" aria-hidden="true" />
              {item.email_count}
            </span>
          {/if}
          {#if item.last_activity_at}
            <span class="ml-auto shrink-0 text-xs text-muted-foreground sm:ml-0 sm:w-24 sm:text-right">{timeAgo(item.last_activity_at, locale())}</span>
          {/if}
        </div>
      </li>
    {/each}
  </ul>
  {#if items.length > limit}
    <!-- Remounted per page: on a viewport taller than a page the sentinel is still in
         view after the rows land, and only a fresh observer reports that. -->
    {#key limit}
      <InfiniteScroll enabled onLoad={showMoreFromSentinel} />
    {/key}
    <!-- The control InfiniteScroll leaves to its caller, for whoever moves by keyboard or
         reads with a screen reader rather than scrolls. Hidden until it takes focus: the
         list reads as one continuous list for everyone else. -->
    <div bind:this={fallback} class="sr-only focus-within:not-sr-only">
      <LoadMore loading={false} onclick={showMoreFromKeyboard} />
    </div>
  {/if}
{/if}
