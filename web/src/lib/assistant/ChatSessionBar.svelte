<script lang="ts">
  import type { Snippet } from 'svelte';
  import { PanelLeft, Plus, Trash2 } from '@lucide/svelte';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { Button } from '#lib/ui/index.js';
  import type { SessionItem } from './sessions';
  import { messages } from './AssistantChat.messages';

  // AssistantChat's header row over the conversation where the session rail exists: on
  // desktop the control that folds the rail (`sidebarOpen`), on phones a compact switcher in
  // the rail's place. `navTrigger` is the host's own account-navigation control.
  let {
    sessions,
    activeId,
    switching,
    newDisabled,
    sidebarOpen = $bindable(),
    navTrigger,
    onNew,
    onSelect,
    onDelete,
  }: {
    sessions: SessionItem[];
    activeId: string | null;
    switching: boolean;
    newDisabled: boolean;
    sidebarOpen: boolean;
    navTrigger?: Snippet;
    onNew: () => void;
    onSelect: (id: string) => void;
    onDelete: (id: string) => void;
  } = $props();

  const s = $derived(t(messages, locale()));
</script>

<!-- Desktop: collapse/expand the session list (only when a rail exists). -->
<div class="hidden items-center gap-1 border-b border-border px-2 py-1.5 md:flex">
  {@render navTrigger?.()}
  <button
    type="button"
    onclick={() => (sidebarOpen = !sidebarOpen)}
    class="rounded p-1 text-muted-foreground transition-colors hover:text-foreground"
    aria-label={sidebarOpen ? s.hideChats : s.showChats}
    title={sidebarOpen ? s.hideChats : s.showChats}
  >
    <PanelLeft class="size-4" />
  </button>
  {#if !sidebarOpen}
    <button
      type="button"
      onclick={onNew}
      disabled={newDisabled}
      class="flex items-center gap-1.5 rounded px-1.5 py-1 text-sm text-muted-foreground transition-colors hover:text-foreground disabled:cursor-not-allowed disabled:opacity-50"
      title={s.newChat}
    >
      <Plus class="size-4" />{s.newChat}
    </button>
  {/if}
</div>
<!-- Mobile session switcher -->
<!-- One compact bar, every control at the design system's h-8: it is chrome above the
 conversation, and at 44px with a filled brand button it read as the page's main
 action and took ~60px of a phone's height. "New" stays labelled rather than a
 bare glyph. Delete is pushed away with a gap: an unlabelled trash icon flush
 against the "new" control is a mis-tap that destroys a conversation. -->
<div class="flex items-center gap-2 border-b border-border px-2 py-1.5 md:hidden">
  {@render navTrigger?.()}
  <select
    value={activeId}
    onchange={(e) => onSelect(e.currentTarget.value)}
    disabled={switching}
    aria-label={s.selectChat}
    class="h-8 min-w-0 flex-1 rounded-md border border-border bg-background px-2 text-sm"
  >
    {#each sessions as sess (sess.id)}
      <option value={sess.id}>{sess.label}</option>
    {/each}
  </select>
  <Button
    variant="outline"
    size="sm"
    class="shrink-0 gap-1.5 px-2.5"
    onclick={onNew}
    disabled={newDisabled}
    title={s.newChat}
  >
    <Plus class="size-4" aria-hidden="true" />
    {s.newChatShort}
  </Button>
  {#if activeId}
    <Button
      variant="ghost"
      size="icon"
      class="ml-1 shrink-0 text-muted-foreground"
      onclick={() => onDelete(activeId as string)}
      aria-label={s.deleteChatAria}
      title={s.deleteChatAria}
    >
      <Trash2 class="size-4" aria-hidden="true" />
    </Button>
  {/if}
</div>
