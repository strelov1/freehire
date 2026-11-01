<script lang="ts">
  import { onDestroy, type Snippet } from 'svelte';
  import { api } from '#lib/api.js';
  import { portal } from '#lib/actions/portal.js';
  import type { MyJob } from '#lib/types.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { messages } from './BoardPreview.messages';

  let { item, children, class: className }: { item: MyJob; children: Snippet; class?: string } = $props();
  let trigger = $state<HTMLDivElement | null>(null);
  let visible = $state(false);
  let pointerPressed = $state(false);
  let text = $state<string | null>(null);
  let loading = $state(false);
  let left = $state(0);
  let top = $state(0);
  let timer: ReturnType<typeof setTimeout> | undefined;
  const id = $props.id();
  const s = $derived(t(messages, locale()));

  async function show() {
    clearTimeout(timer);
    if (!item.job || !trigger) return;
    const rect = trigger.getBoundingClientRect();
    left = Math.max(8, Math.min(rect.left, window.innerWidth - 328));
    top = Math.max(8, Math.min(rect.bottom + 6, window.innerHeight - 160));
    visible = true;
    if (text !== null || loading) return;
    loading = true;
    try {
      const app = await api.getTrackedApplication(item.job.public_slug);
      const doc = new DOMParser().parseFromString(app.job?.description ?? '', 'text/html');
      doc.querySelectorAll('script, style').forEach((node) => node.remove());
      doc.querySelectorAll('p, div, li, br, h1, h2, h3, h4, tr').forEach((node) => node.append(' '));
      const description = (doc.body.textContent ?? '').replace(/\s+/g, ' ').trim();
      const characters = Array.from(description);
      text = characters.length > 200 ? characters.slice(0, 200).join('') + '…' : description;
    } catch {
      // Keep null so the next deliberate hover/focus can retry.
    } finally {
      loading = false;
    }
  }

  function hide() {
    clearTimeout(timer);
    visible = false;
  }
  function scheduleHide() {
    clearTimeout(timer);
    timer = setTimeout(hide, 150);
  }
  function pointerDown() {
    pointerPressed = true;
    hide();
  }
  function focusIn() {
    if (!pointerPressed) void show();
  }
  function scheduleShow() {
    clearTimeout(timer);
    timer = setTimeout(() => void show(), 300);
  }
  onDestroy(() => clearTimeout(timer));

  // Preserve the card's stage/location descriptions while adding this temporary one.
  $effect(() => {
    if (!visible) return;
    const target = trigger?.querySelector<HTMLElement>('button, [role="button"]');
    const previous = target?.getAttribute('aria-describedby');
    target?.setAttribute('aria-describedby', [previous, id].filter(Boolean).join(' '));
    document.addEventListener('scroll', hide, true);
    return () => {
      document.removeEventListener('scroll', hide, true);
      if (previous) target?.setAttribute('aria-describedby', previous);
      else target?.removeAttribute('aria-describedby');
    };
  });
</script>

<svelte:window
  onkeydown={visible ? (e) => { if (e.key === 'Escape') hide(); } : undefined}
  onresize={visible ? hide : undefined}
  onpointerup={pointerPressed ? () => { pointerPressed = false; } : undefined}
  onpointercancel={pointerPressed ? () => { pointerPressed = false; } : undefined}
/>

<!-- The wrapper delegates hover and keyboard focus to its interactive child. -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div bind:this={trigger} class={className} onpointerenter={(e) => { if (e.pointerType === 'mouse') scheduleShow(); }} onmouseleave={scheduleHide} onfocusin={focusIn} onfocusout={hide} onpointerdown={pointerDown}>
  {@render children()}
</div>
{#if visible}
  <!-- Portalled so a column's scroll container cannot clip the preview. -->
  <div {@attach portal()} {id} role="tooltip"
    onmouseenter={() => clearTimeout(timer)} onmouseleave={scheduleHide}
    class="fixed z-popover w-80 max-w-11/12 rounded-md border border-border bg-popover p-3 text-sm text-popover-foreground shadow-md"
    style:left="{left}px" style:top="{top}px"
  >
    {loading ? s.loading : text || s.unavailable}
  </div>
{/if}
