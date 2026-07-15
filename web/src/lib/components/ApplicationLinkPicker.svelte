<script lang="ts">
  import { Search } from '@lucide/svelte';

  interface PickerApp {
    slug: string;
    company: string;
    title?: string;
  }

  let {
    applications,
    onpick,
    loading = false,
  }: {
    applications: PickerApp[];
    onpick: (slug: string) => void;
    loading?: boolean;
  } = $props();

  let open = $state(false);
  let q = $state('');

  const filtered = $derived.by(() => {
    const needle = q.trim().toLowerCase();
    if (!needle) return applications;
    return applications.filter((a) => `${a.company} ${a.title ?? ''}`.toLowerCase().includes(needle));
  });

  function pick(slug: string) {
    onpick(slug);
    open = false;
    q = '';
  }
</script>

<div class="relative inline-block">
  <button
    type="button"
    onclick={() => (open = !open)}
    class="inline-flex items-center gap-1 rounded-full border border-dashed border-border px-2.5 py-0.5 text-xs text-muted-foreground transition-colors hover:border-brand-ring hover:text-foreground"
  >
    Link to application <span aria-hidden="true">▾</span>
  </button>

  {#if open}
    <!-- click-catcher: closing on any outside click -->
    <button
      type="button"
      class="fixed inset-0 z-10 cursor-default"
      aria-label="Close application picker"
      onclick={() => (open = false)}
    ></button>

    <div
      class="absolute left-0 z-20 mt-1 w-64 overflow-hidden rounded-md border border-border bg-background shadow-md"
    >
      {#if loading}
        <p class="p-3 text-xs text-muted-foreground">Loading your applications…</p>
      {:else if applications.length === 0}
        <p class="p-3 text-xs text-muted-foreground">No applications yet — track a job first.</p>
      {:else}
        <div class="flex items-center gap-1.5 border-b border-border px-2.5 py-1.5">
          <Search class="size-3.5 shrink-0 text-muted-foreground" />
          <input
            bind:value={q}
            placeholder="Search applications…"
            class="w-full bg-transparent text-xs outline-none placeholder:text-muted-foreground"
          />
        </div>
        <ul class="max-h-56 overflow-y-auto py-1">
          {#each filtered as a (a.slug)}
            <li>
              <button
                type="button"
                onclick={() => pick(a.slug)}
                class="flex w-full flex-col items-start px-2.5 py-1.5 text-left text-xs transition-colors hover:bg-muted"
              >
                <span class="font-medium text-foreground">{a.company || 'Unknown company'}</span>
                {#if a.title}<span class="truncate text-muted-foreground">{a.title}</span>{/if}
              </button>
            </li>
          {:else}
            <li class="px-2.5 py-1.5 text-xs text-muted-foreground">No match.</li>
          {/each}
        </ul>
      {/if}
    </div>
  {/if}
</div>
