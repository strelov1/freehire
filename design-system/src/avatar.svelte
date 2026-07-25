<script lang="ts">
  import { cn } from './cn.js';

  let {
    name,
    src,
    size = 'md',
    class: className,
  }: {
    name?: string;
    src?: string;
    size?: 'sm' | 'md' | 'lg';
    class?: string;
  } = $props();

  const sizes = {
    sm: 'size-8 text-xs',
    md: 'size-10 text-sm',
    lg: 'size-12 text-base',
  };

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
          .split(' ')
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
</script>

{#if src}
  <img {src} alt={name ?? ''} class={cn('rounded-full object-cover', sizes[size], className)} />
{:else}
  <!-- role=img so the label is honoured — a bare <div> is role=generic, which
       prohibits naming, and the initials would be spelled out instead of the
       name. Without a name it conveys nothing, so keep it out of the tree. -->
  <div
    class={cn('flex items-center justify-center rounded-full font-medium', sizes[size], className)}
    style="background-color: {bg}; color: {fg}"
    role={name ? 'img' : undefined}
    aria-label={name}
    aria-hidden={name ? undefined : true}
  >
    {initials}
  </div>
{/if}
