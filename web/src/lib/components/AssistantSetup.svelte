<script lang="ts">
  import McpSetup from './McpSetup.svelte';
  import CliSetup from './CliSetup.svelte';

  // The setup half of /mcp: MCP or CLI, then the assistant.
  let view = $state<'mcp' | 'cli'>('mcp');
</script>

<div class="mb-4 flex justify-end">
  <div class="inline-flex rounded-full border border-border p-1" role="group" aria-label="Setup method">
    {#each ['mcp', 'cli'] as v (v)}
      <button
        type="button"
        aria-pressed={view === v}
        onclick={() => (view = v as 'mcp' | 'cli')}
        class={[
          'rounded-full px-5 py-1.5 text-sm font-medium uppercase transition-colors',
          view === v ? 'bg-background text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground',
        ]}
      >
        {v}
      </button>
    {/each}
  </div>
</div>

{#if view === 'mcp'}
  <McpSetup />
{:else}
  <CliSetup />
{/if}
