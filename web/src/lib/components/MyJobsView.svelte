<script lang="ts">
  import { isAuthenticated } from '$lib/auth.svelte';
  import { cn } from '$lib/utils';
  import JobBoard from './JobBoard.svelte';
  import JobHistory from './JobHistory.svelte';

  type View = 'board' | 'history';
  const tabs: { value: View; label: string }[] = [
    { value: 'board', label: 'Board' },
    { value: 'history', label: 'History' },
  ];
  let view = $state<View>('board');
</script>

{#if !isAuthenticated()}
  <p class="py-12 text-center text-sm text-muted-foreground">
    Sign in to see the jobs you saved, applied to, and are tracking.
  </p>
{:else}
  <div class="flex flex-col gap-4">
    <h1 class="text-2xl font-semibold tracking-tight">My jobs</h1>

    <div role="tablist" aria-label="My jobs view" class="flex items-center gap-1">
      {#each tabs as tab (tab.value)}
        <button
          type="button"
          role="tab"
          aria-selected={view === tab.value}
          onclick={() => (view = tab.value)}
          class={cn(
            'rounded-md px-3 py-1.5 text-sm transition-colors',
            view === tab.value
              ? 'bg-secondary font-medium text-secondary-foreground'
              : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
          )}
        >
          {tab.label}
        </button>
      {/each}
    </div>

    {#if view === 'board'}
      <JobBoard />
    {:else}
      <JobHistory />
    {/if}
  </div>
{/if}
