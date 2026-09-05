<script lang="ts">
  import { ChevronRight, Terminal } from '@lucide/svelte';
  import {
    groupTitle,
    isExpandable,
    callLine,
    nonEmptyInput,
    previewToolInput,
    toolErrorMessage,
    parseConfirmationRequest,
    CONFIRMATION_DECLINE_TEXT,
    type ToolCall,
  } from '$lib/assistant/tool-formatters';

  // The tool calls of one assistant message, rendered as collapsed cards. The
  // agent's tools are typed functions rather than shell commands, so there is one
  // shape for all of them: an intent label, the argument that identifies the call,
  // and — when it failed — the reason the model was given.
  //
  // `request_confirmation` is the one exception: instead of a collapsed line it
  // renders as the claim text plus Yes/No. `onConfirm` sends whichever text the
  // candidate picked as an ordinary chat message — clicking Yes replays the claim
  // verbatim, which is what lets the backend's own verbatim-quote check recognise it
  // as the candidate's own words. There is no other wiring: this component holds no
  // state about which claims were already answered.
  let {
    calls,
    onConfirm,
    disabled = false,
  }: { calls: readonly ToolCall[]; onConfirm: (text: string) => void; disabled?: boolean } = $props();

  // Fold the flat list into consecutive runs of the same tool, so a burst of
  // searches reads as one card rather than five.
  function groupTools(flat: readonly ToolCall[]): ToolCall[][] {
    const groups: ToolCall[][] = [];
    for (const c of flat) {
      const last = groups[groups.length - 1];
      if (last && last[0]?.name === c.name) last.push(c);
      else groups.push([c]);
    }
    return groups;
  }

  // One chip shape for both the flat card and the expandable summary, so the two never
  // drift apart by a padding step. The expandable one adds only its hover affordance.
  const chip =
    'self-start inline-flex items-center gap-2 rounded-lg border border-border/60 bg-muted/40 px-2.5 py-1.5 text-sm leading-5 text-muted-foreground';
</script>

{#each groupTools(calls) as g, t (t)}
  {@const title = groupTitle(g)}
  {#if g[0]?.name === 'request_confirmation'}
    {#each g as c, ci (ci)}
      {@const confirmation = parseConfirmationRequest(c)}
      {#if confirmation}
        <div class="self-start max-w-[90%] flex flex-col gap-2 rounded-lg border border-border bg-muted/50 px-3 py-2.5 text-sm">
          {#if confirmation.question}
            <p class="text-muted-foreground">{confirmation.question}</p>
          {/if}
          <p class="font-medium text-foreground">{confirmation.claim}</p>
          <div class="flex gap-2">
            <button
              type="button"
              onclick={() => onConfirm(confirmation.claim)}
              {disabled}
              class="rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
            >
              Yes
            </button>
            <button
              type="button"
              onclick={() => onConfirm(CONFIRMATION_DECLINE_TEXT)}
              {disabled}
              class="rounded-md border border-border px-3 py-1.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted disabled:opacity-50"
            >
              No
            </button>
          </div>
        </div>
      {/if}
    {/each}
  {:else if !isExpandable(g)}
    <div class={chip}>
      <Terminal class="size-3.5 shrink-0 opacity-60" />
      <span class="font-medium">{title}</span>
    </div>
  {:else}
    <!-- `group` so the chevron can read the details' own open state; a variant hung on
         the summary cannot — `[open]` sits on the <details>, never on its summary. -->
    <details class="group self-start max-w-[90%]">
      <summary
        class="{chip} cursor-pointer transition-colors hover:border-border hover:bg-muted/70 [&::-webkit-details-marker]:hidden [&::marker]:hidden"
      >
        <Terminal class="size-3.5 shrink-0 opacity-60" />
        <span class="font-medium">{title}</span>
        <ChevronRight class="size-3.5 shrink-0 opacity-50 transition-transform group-open:rotate-90" />
      </summary>
      <ul class="mt-1.5 ml-5 space-y-1 text-xs text-muted-foreground">
        {#each g as c, ci (ci)}
          <li class="flex flex-wrap items-baseline gap-1.5">
            <span class={c.isError ? 'text-destructive' : ''}>{callLine(c)}</span>
            {#if toolErrorMessage(c)}
              <span class="text-destructive">— {toolErrorMessage(c)}</span>
            {:else if nonEmptyInput(c.input)}
              <code class="rounded bg-muted px-1.5 py-0.5 font-mono">{previewToolInput(c.input)}</code>
            {/if}
          </li>
        {/each}
      </ul>
    </details>
  {/if}
{/each}
