<script lang="ts">
  import { cn } from './cn.js';

  // Deterministic color from a string — uses the hue range of the brand
  // palette so avatars feel on-brand rather than random rainbow.
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

  let bg = $derived(
    name
      ? `hsl(${hashHue(name)} 45% 90%)`
      : 'hsl(0 0% 90%)',
  );
</script>

{#if src}
  <img {src} alt={name ?? 'avatar'} class={cn('rounded-full object-cover', sizes[size], className)} />
{:else}
  <div
    class={cn('flex items-center justify-center rounded-full font-medium text-foreground', sizes[size], className)}
    style="background-color: {bg}"
    aria-label={name ?? 'avatar'}
  >
    {initials}
  </div>
{/if}
