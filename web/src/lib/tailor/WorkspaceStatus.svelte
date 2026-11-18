<script lang="ts">
  import { resolve } from '$app/paths';

  // The tailoring workspace before it is usable: opening, or why it could not open and the
  // way back out.
  let {
    status,
    baseMode,
    resuming,
    errorMsg,
    slug,
  }: {
    status: 'loading' | 'error';
    baseMode: boolean;
    resuming: boolean;
    errorMsg: string;
    slug: string;
  } = $props();
</script>

{#if status === 'loading'}
  <div class="flex min-w-0 flex-1 items-center justify-center text-sm text-muted-foreground">
    {baseMode
      ? 'Opening your CV…'
      : resuming
        ? 'Re-opening your tailoring session…'
        : 'Preparing your tailoring session…'}
  </div>
{:else if status === 'error'}
  <div class="flex min-w-0 flex-1 flex-col items-center justify-center gap-3 p-6 text-center">
    <p class="max-w-md text-sm text-destructive">{errorMsg}</p>
    {#if baseMode}
      <a href={resolve('/my/cvs')} class="text-sm text-brand hover:underline">Back to your CVs</a>
    {:else}
      <a href={resolve('/jobs/[slug]', { slug })} class="text-sm text-brand hover:underline">Back to the role</a>
    {/if}
  </div>
{/if}
