<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { Button } from '#lib/ui/index.js';
</script>

<div class="mx-auto w-full max-w-3xl px-4 py-16 text-center">
  <h1 class="text-2xl font-semibold tracking-tight">{page.status}</h1>
  <p class="mt-2 text-sm text-muted-foreground">
    {page.error?.message ?? 'Something went wrong.'}
  </p>
  <div class="mt-4 flex items-center justify-center gap-4">
    <!-- A failed page start (stale cached scripts, a dropped connection) usually clears on
         a fresh load, so offer one instead of leaving the visitor at a dead end. -->
    {#if page.status >= 500}
      <Button variant="primary" onclick={() => location.reload()}>Reload</Button>
    {/if}
    <a href={resolve('/jobs')} class="text-sm font-medium text-foreground hover:underline">Browse jobs</a>
  </div>
  {#if page.error?.id}
    <p class="mt-6 text-xs text-muted-foreground">
      Error code <code class="font-mono select-all">{page.error.id}</code>
    </p>
  {/if}
</div>
