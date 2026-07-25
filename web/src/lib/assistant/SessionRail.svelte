<script lang="ts">
  import { Plus, Trash2 } from '@lucide/svelte';
  import type { SessionItem } from '$lib/assistant/sessions';

  // Session rail (desktop): the collapsible chat list. Pure rendering — session
  // orchestration (create/select/delete) stays with the parent and arrives as
  // callbacks; `ready` stands in for the parent's connection phase.
  let {
    sessions,
    activeId,
    switching,
    ready,
    onNew,
    onSelect,
    onDelete,
  }: {
    sessions: SessionItem[];
    activeId: string | null;
    switching: boolean;
    ready: boolean;
    onNew: () => void;
    onSelect: (id: string) => void;
    onDelete: (id: string) => void;
  } = $props();
</script>

<aside class="hidden w-64 shrink-0 flex-col border-r border-border bg-muted/20 md:flex">
  <div class="p-2">
    <button
      type="button"
      onclick={onNew}
      disabled={switching || !ready}
      class="flex w-full items-center gap-2 rounded-lg border border-border px-3 py-2 text-sm font-medium transition-colors hover:bg-muted disabled:cursor-not-allowed disabled:opacity-50"
    >
      <Plus class="size-4" />
      New chat
    </button>
  </div>
  <ul class="flex-1 space-y-1 overflow-y-auto px-2 pb-2">
    {#each sessions as s (s.id)}
      <li class="group relative">
        <button
          type="button"
          onclick={() => onSelect(s.id)}
          disabled={switching}
          class={[
            'flex w-full items-center rounded-lg py-2 pl-3 pr-9 text-left text-sm transition-colors',
            s.id === activeId ? 'bg-secondary text-secondary-foreground' : 'hover:bg-muted',
          ]}
        >
          <span class="min-w-0 flex-1 truncate">{s.label}</span>
        </button>
        <button
          type="button"
          aria-label="Delete chat"
          title="Delete chat"
          onclick={() => onDelete(s.id)}
          class="absolute right-1 top-1/2 flex size-7 -translate-y-1/2 items-center justify-center rounded-md text-muted-foreground opacity-0 transition-opacity hover:bg-muted hover:text-foreground focus-visible:opacity-100 group-hover:opacity-100"
        >
          <Trash2 class="size-4" />
        </button>
      </li>
    {/each}
  </ul>
</aside>
