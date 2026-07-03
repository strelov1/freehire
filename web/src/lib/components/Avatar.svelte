<script lang="ts">
  import { cn } from '$lib/utils';

  // A placeholder avatar: a circle showing the email's first character on a colour
  // deterministically derived from the email, so it is stable per user with no uploaded
  // image. Full class strings are listed statically so Tailwind's JIT keeps them.
  let { email, class: klass = '' }: { email: string; class?: string } = $props();

  const palette = [
    'bg-red-500',
    'bg-orange-500',
    'bg-amber-500',
    'bg-green-500',
    'bg-teal-500',
    'bg-blue-500',
    'bg-indigo-500',
    'bg-violet-500',
    'bg-pink-500',
    'bg-rose-500',
  ] as const;

  const initial = $derived((email.trim()[0] ?? '?').toUpperCase());
  // Sum the char codes for a cheap, stable hash, then index the palette.
  const color = $derived.by(() => {
    let sum = 0;
    for (const ch of email) sum += ch.charCodeAt(0);
    return palette[sum % palette.length];
  });
</script>

<span
  aria-hidden="true"
  class={cn(
    'flex size-7 shrink-0 items-center justify-center rounded-full text-xs font-semibold text-white',
    color,
    klass,
  )}
>
  {initial}
</span>
