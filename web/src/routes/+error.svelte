<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
</script>

<div class="mx-auto w-full max-w-3xl px-4 py-16 text-center">
  <h1 class="text-2xl font-semibold tracking-tight">{page.status}</h1>
  <p class="mt-2 text-sm text-muted-foreground">
    {page.error?.message ?? 'Something went wrong.'}
  </p>
  <a href={resolve('/jobs')} class="mt-4 inline-block text-sm font-medium text-foreground hover:underline">
    Browse jobs
  </a>
</div>
