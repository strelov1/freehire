<script lang="ts">
  import { resolve } from '$app/paths';
  import { Check, Link, Share, Share2 } from '@lucide/svelte';
  import { menuNav } from '#lib/actions/menuNav.js';

  let { slug, title, compact = false }: { slug: string; title: string; compact?: boolean } = $props();

  let root: HTMLDivElement;
  let open = $state(false);
  let canShare = $state(false);
  let feedback = $state('');
  const copied = $derived(feedback === 'Link copied');

  const menuId = $props.id();
  const rowClass =
    'flex min-h-10 w-full items-center gap-2.5 rounded-md px-2.5 text-left text-sm font-medium hover:bg-accent hover:text-accent-foreground focus-visible:bg-accent focus-visible:text-accent-foreground';

  function shareData() {
    const path = resolve('/jobs/[slug]', { slug });
    return { title, url: new URL(path, window.location.href).href };
  }

  function close() {
    open = false;
  }

  function toggle(event: MouseEvent) {
    if (open) {
      close();
      return;
    }
    (event.currentTarget as HTMLButtonElement).focus();
    canShare = typeof navigator.share === 'function' && (navigator.canShare?.(shareData()) ?? true);
    feedback = '';
    open = true;
  }

  async function share() {
    try {
      await navigator.share(shareData());
      close();
    } catch (error) {
      if (!(error instanceof DOMException && error.name === 'AbortError')) {
        feedback = 'Sharing unavailable. Use Copy link.';
      }
    }
  }

  async function copyLink() {
    try {
      await navigator.clipboard.writeText(shareData().url);
      feedback = 'Link copied';
    } catch {
      feedback = 'Could not copy the link.';
    }
  }

  $effect(() => {
    if (!open) return;

    function dismissOutside(event: PointerEvent) {
      if (event.target instanceof Node && !root.contains(event.target)) close();
    }

    function dismissOnEscape(event: KeyboardEvent) {
      if (event.key === 'Escape') close();
    }

    window.addEventListener('pointerdown', dismissOutside);
    window.addEventListener('keydown', dismissOnEscape);
    return () => {
      window.removeEventListener('pointerdown', dismissOutside);
      window.removeEventListener('keydown', dismissOnEscape);
    };
  });
</script>

<div class="relative inline-flex shrink-0" bind:this={root}>
  <button
    type="button"
    aria-label="Share this job"
    title="Share this job"
    aria-haspopup="menu"
    aria-expanded={open}
    aria-controls={open ? menuId : undefined}
    onclick={toggle}
    class={[
      'grid shrink-0 place-items-center rounded-lg text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:ring-2 focus-visible:ring-ring',
      compact ? 'size-8' : 'size-9',
      open && 'bg-accent text-accent-foreground',
    ]}
  >
    <Share2 class="size-4" aria-hidden="true" />
  </button>

  <span class="sr-only" role="status">{open ? feedback : ''}</span>

  {#if open}
    <div
      id={menuId}
      role="menu"
      aria-label="Share this job"
      {@attach menuNav({ onclose: close })}
      class="absolute top-full right-0 z-popover mt-1 flex w-max min-w-36 flex-col rounded-lg border border-border bg-popover p-1 text-popover-foreground shadow-md"
    >
      {#if canShare}
        <button type="button" role="menuitem" tabindex="-1" class={rowClass} onclick={share}>
          <Share class="size-4" aria-hidden="true" />
          Share
        </button>
      {/if}
      <button type="button" role="menuitem" tabindex="-1" class={rowClass} onclick={copyLink}>
        {#if copied}
          <Check class="size-4" aria-hidden="true" />
          Link copied
        {:else}
          <Link class="size-4" aria-hidden="true" />
          Copy link
        {/if}
      </button>
      {#if feedback && !copied}
        <p class="max-w-44 px-2.5 py-1 text-xs text-muted-foreground">{feedback}</p>
      {/if}
    </div>
  {/if}
</div>
