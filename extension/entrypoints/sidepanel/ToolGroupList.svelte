<script lang="ts">
  import { ChevronRight, Terminal } from '@lucide/svelte';
  import {
    groupTitle,
    isExpandable,
    callLine,
    nonEmptyInput,
    previewToolInput,
    toolErrorMessage,
    type ToolCall,
  } from '../../lib/assistant/tool-formatters';
  import { pageReadTarget } from '../../lib/assistant/pageRead';

  // The tool calls of one assistant message, rendered as collapsed rows. Ported
  // from the web app's `ToolGroupList.svelte`; the formatting logic is shared
  // verbatim through `tool-formatters` and the two draw the same icons, only the
  // styling differs — the web has Tailwind's design tokens, this panel is 400px
  // wide and styles itself.
  let { calls }: { calls: readonly ToolCall[] } = $props();

  // Fold the flat list into consecutive runs of the same tool, so a burst of
  // searches reads as one row rather than five.
  function groupTools(flat: readonly ToolCall[]): ToolCall[][] {
    const groups: ToolCall[][] = [];
    for (const c of flat) {
      const last = groups[groups.length - 1];
      if (last && last[0]?.name === c.name) last.push(c);
      else groups.push([c]);
    }
    return groups;
  }
</script>

{#each groupTools(calls) as g, t (t)}
  {@const title = groupTitle(g)}
  {#if !isExpandable(g)}
    <div class="tool">
      <Terminal class="size-3.5 shrink-0 opacity-60" />
      <span class="title">{title}</span>
    </div>
  {:else}
    <details class="tool">
      <summary>
        <Terminal class="size-3.5 shrink-0 opacity-60" />
        <span class="title">{title}</span>
        <ChevronRight class="chev size-3.5 shrink-0 opacity-50 transition-transform" />
      </summary>
      <ul>
        {#each g as c, ci (ci)}
          <li>
            <span class:err={c.isError}>{callLine(c)}</span>
            {#if pageReadTarget(c)}
              <!-- Which page was read, so a read the agent chose to make is one the
                   user can see. Query and fragment are dropped upstream. -->
              <code class="page">{pageReadTarget(c)}</code>
            {/if}
            {#if toolErrorMessage(c)}
              <span class="err">— {toolErrorMessage(c)}</span>
            {:else if nonEmptyInput(c.input)}
              <code>{previewToolInput(c.input)}</code>
            {/if}
          </li>
        {/each}
      </ul>
    </details>
  {/if}
{/each}

<style>
  .tool {
    align-self: flex-start;
    max-width: 90%;
    font-size: 12px;
    line-height: 18px;
    color: var(--muted-foreground);
    background: var(--muted);
    border: 1px solid var(--border);
    border-radius: 8px;
  }

  /* The summary's hover fill runs to the card's edge; without this it squares off the
     two top corners. Only the expandable card has a summary to fill. */
  details.tool {
    overflow: hidden;
  }

  /* The padding sits on the row rather than the card so that an expanded card's list
     is not indented by it as well. */
  div.tool,
  summary {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 10px;
  }

  .title {
    font-weight: 500;
  }

  summary {
    cursor: pointer;
    list-style: none;
    transition: background 120ms;
  }

  summary:hover {
    background: var(--background);
  }

  summary::-webkit-details-marker {
    display: none;
  }

  /* A disclosure the user can miss is a disclosure they will not open.
     :global — the class is forwarded onto the icon component's own <svg>, which this
     file's scope does not reach. */
  details[open] :global(.chev) {
    transform: rotate(90deg);
  }

  ul {
    margin: 0;
    padding: 0 10px 8px 24px;
    font-size: 11px;
    line-height: 1.5;
  }

  li {
    word-break: break-word;
  }

  code {
    background: var(--background);
    border-radius: 4px;
    padding: 1px 4px;
    font-family: ui-monospace, monospace;
  }

  .err {
    color: var(--destructive);
  }

  /* The page a read touched: quiet, but legible enough to notice a surprise. */
  .page {
    color: var(--muted-foreground);
    word-break: break-all;
  }
</style>
