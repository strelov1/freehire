<script lang="ts">
  import { Terminal, ChevronRight } from '@lucide/svelte';
  import {
    classifyFamily,
    groupTitle,
    isExpandable,
    callLine,
    bashCommand,
    commandLine,
    isFreehireGroup,
    isNoiseShellCall,
    nonEmptyInput,
    previewToolInput,
    type ToolCall,
    type ToolFamily,
  } from '$lib/assistant/tool-formatters';

  // One message's tool calls, rendered as consecutive same-family groups (one
  // card per run of e.g. bash commands or file reads). Pure presentation.
  let { calls }: { calls: readonly ToolCall[] } = $props();

  // Fold a message's flat tool-call list into consecutive same-family groups,
  // so the renderer shows one card per run of e.g. bash commands or file reads.
  function groupTools(flat: readonly ToolCall[]): { family: ToolFamily; calls: ToolCall[] }[] {
    const groups: { family: ToolFamily; calls: ToolCall[] }[] = [];
    for (const c of flat) {
      const family = classifyFamily(c);
      const last = groups[groups.length - 1];
      if (last && last.family === family) last.calls.push(c);
      else groups.push({ family, calls: [c] });
    }
    return groups;
  }
</script>

{#each groupTools(calls.filter((c) => !isNoiseShellCall(c))) as g, t (t)}
  {@const title = groupTitle(g.family, g.calls)}
  {#if !isExpandable(g.family, g.calls)}
    <div class="self-start flex items-center gap-2 rounded-lg border border-border bg-muted/50 px-3 py-2 text-sm text-muted-foreground">
      <Terminal class="size-4 shrink-0" />
      <span>{title}</span>
    </div>
  {:else}
    <details class="self-start max-w-[90%]">
      <summary class="flex cursor-pointer items-center gap-2 rounded-lg border border-border bg-muted/50 px-3 py-2 text-sm text-muted-foreground hover:bg-muted/70 [&::-webkit-details-marker]:hidden [&::marker]:hidden [&[open]>span>svg.chev]:rotate-90">
        <Terminal class="size-4 shrink-0" />
        <span class="flex items-center gap-1.5">
          {title}
          <ChevronRight class="chev size-3.5 shrink-0 transition-transform" />
        </span>
      </summary>
      {#if g.family === 'bash' && isFreehireGroup(g.calls)}
        <ul class="mt-2 ml-6 space-y-1 text-xs text-muted-foreground">
          {#each g.calls as c, ci (ci)}
            <li>{commandLine(c)}</li>
          {/each}
        </ul>
      {:else if g.family === 'bash'}
        <div class="mt-2 overflow-hidden rounded-md border border-border bg-background">
          <div class="border-b border-border bg-muted/40 px-3 py-1.5 text-[0.65rem] font-medium uppercase tracking-wider text-muted-foreground/80">
            Shell
          </div>
          {#each g.calls as c, ci (ci)}
            <pre class={['m-0 whitespace-pre-wrap break-words px-3 py-2 font-mono text-xs', ci > 0 && 'border-t border-border']}>$ {bashCommand(c.input) ?? ''}</pre>
          {/each}
        </div>
      {:else if g.family === 'fs'}
        <ul class="mt-2 ml-6 space-y-1 text-xs text-muted-foreground">
          {#each g.calls as c, ci (ci)}
            <li>{callLine(c)}</li>
          {/each}
        </ul>
      {:else}
        <ul class="mt-2 ml-6 space-y-1 text-xs text-muted-foreground">
          {#each g.calls as c, ci (ci)}
            <li class="flex flex-wrap items-baseline gap-1.5">
              <span class="font-medium text-foreground/80">{c.name}</span>
              {#if nonEmptyInput(c.input)}
                <code class="rounded bg-muted px-1.5 py-0.5 font-mono">{previewToolInput(c.input)}</code>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
    </details>
  {/if}
{/each}
