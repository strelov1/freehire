<script lang="ts">
  import type { Snippet } from 'svelte';
  import { cn } from './cn.js';

  let {
    name,
    src,
    size = 'md',
    shape = 'circle',
    fallbackIcon,
    class: className,
  }: {
    name?: string;
    src?: string;
    size?: 'xs' | 'sm' | 'md' | 'lg';
    /** `square` is a rounded tile (a company/entity mark); `circle` (default) is a person avatar. */
    shape?: 'circle' | 'square';
    /** Shown only when there is neither a photo nor a name to derive initials from. */
    fallbackIcon?: Snippet;
    class?: string;
  } = $props();

  const sizes = {
    xs: 'size-5 text-[9px]',
    sm: 'size-8 text-xs',
    md: 'size-10 text-sm',
    lg: 'size-12 text-base',
  };

  const shapeClass = $derived(shape === 'square' ? 'rounded' : 'rounded-full');

  // Deterministic hue from the name, across the full circle — what keeps the
  // result calm is the fixed low saturation below, not the hue itself. Kept as
  // an inline colour because a token per hue makes no sense.
  function hashHue(s: string): number {
    let h = 0;
    for (let i = 0; i < s.length; i++) {
      h = (h * 31 + s.charCodeAt(i)) | 0;
    }
    return Math.abs(h) % 360;
  }

  let initials = $derived(
    name
      ? name
          .trim()
          .split(/\s+/)
          .slice(0, 2)
          .map((w) => w[0]?.toUpperCase() ?? '')
          .join('')
      : '?',
  );

  // One hue at two lightnesses: both halves fixed, so the pair carries its own
  // contrast (>= 6:1 on every hue) in either theme. Borrowing --foreground for
  // the ink would invert under the dark theme and leave near-white initials on
  // this near-white fill. Nameless falls to grey through the saturation.
  let hue = $derived(name ? hashHue(name) : 0);
  let saturation = $derived(name ? 45 : 0);
  let bg = $derived(`hsl(${hue} ${saturation}% 90%)`);
  let fg = $derived(`hsl(${hue} ${saturation}% 25%)`);

  let failed = $state(false);

  // A new src is a fresh attempt — clear a prior failure when it changes (a call site may
  // reuse this instance across a data refresh rather than remounting it).
  $effect(() => {
    void src;
    failed = false;
  });

  // SSR sends the raw <img>, so the browser fetches it before hydration wires `onerror`. When
  // the src 404s (a genuine miss), that error event fires before the handler exists and is
  // lost — leaving an empty broken image until a reload. On mount, an already-complete image
  // with no intrinsic width is that missed failure, so fall back to the initials render
  // straight away.
  function catchMissedError(node: HTMLImageElement) {
    if (node.complete && node.naturalWidth === 0) failed = true;
  }
</script>

{#if src && !failed}
  <!-- square (the entity-mark shape) is always rendered beside the name as visible text at
       every call site — see CompanyLogo's original alt="" — so it stays decorative there and
       is named only for the circular person-avatar case, which may be the sole identification. -->
  <img
    {src}
    alt={shape === 'square' ? '' : (name ?? '')}
    loading="lazy"
    class={cn(shapeClass, 'object-cover', sizes[size], className)}
    onerror={() => (failed = true)}
    {@attach catchMissedError}
  />
{:else if !name && fallbackIcon}
  <div
    class={cn('flex items-center justify-center text-muted-foreground', shapeClass, sizes[size], className)}
    aria-hidden="true"
  >
    {@render fallbackIcon()}
  </div>
{:else}
  <!-- role=img so the label is honoured — a bare <div> is role=generic, which
       prohibits naming, and the initials would be spelled out instead of the
       name. Without a name it conveys nothing, so keep it out of the tree. -->
  <div
    class={cn('flex items-center justify-center font-medium', shapeClass, sizes[size], className)}
    style="background-color: {bg}; color: {fg}"
    role={name ? 'img' : undefined}
    aria-label={name}
    aria-hidden={name ? undefined : true}
  >
    {initials}
  </div>
{/if}
