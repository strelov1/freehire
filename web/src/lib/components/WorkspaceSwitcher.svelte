<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { Check, Settings } from '@lucide/svelte';
  import { currentUser } from '#lib/auth.svelte.js';
  import { visibleWorkspaces, workspaceFor } from '#lib/accountNav.js';
  import { workspaceIcons } from '#lib/accountNavIcons.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { messages } from '#lib/i18n/shell.js';
  import { t } from '#lib/i18n/t.js';
  import { cn } from '#lib/ui/index.js';

  // The account entry of both header menus: the workspaces a member can be in, then the
  // settings every workspace shares. It deliberately lists no sections — repeating the
  // sidebar item by item is what made the profile menu long, and the sections of each
  // workspace live in its own sidebar. The host draws the rows (`rowClass`) so the
  // desktop panel and the phone drawer keep their own density.

  let { rowClass, onselect }: { rowClass: string; onselect: () => void } = $props();

  const s = $derived(t(messages, locale()));
  const current = $derived(workspaceFor(page.url.pathname));
  const workspaceList = $derived(visibleWorkspaces(currentUser()?.beta_tester ?? false));
</script>

<div class="px-3 pb-1 pt-2 text-xs font-medium uppercase tracking-wide text-muted-foreground/70">
  {s.shell.workspace}
</div>
{#each workspaceList as w (w.id)}
  {@const Icon = workspaceIcons[w.id]}
  <a
    href={resolve(w.home)}
    role="menuitem"
    aria-current={w.id === current ? 'true' : undefined}
    onclick={onselect}
    class={cn(rowClass, 'items-start py-2')}
  >
    <Icon class="mt-0.5 size-4 shrink-0" />
    <span class="flex min-w-0 flex-1 flex-col">
      <span class={cn(w.id === current && 'font-medium text-foreground')}>{s.workspaces[w.id]}</span>
      <span class="text-xs text-muted-foreground/70">{s.workspaceHints[w.id]}</span>
    </span>
    {#if w.id === current}
      <Check class="mt-0.5 size-4 shrink-0 text-foreground" aria-hidden="true" />
    {/if}
  </a>
{/each}
<div class="my-1 h-px shrink-0 bg-border"></div>
<a href={resolve('/my/account')} role="menuitem" onclick={onselect} class={rowClass}>
  <Settings class="size-4 shrink-0" />
  {s.shell.settings}
</a>
