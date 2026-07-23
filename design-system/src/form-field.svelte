<script lang="ts">
  import type { Snippet } from 'svelte';
  import { cn } from './cn.js';

  let {
    label,
    error,
    hint,
    required = false,
    class: className,
    children,
  }: {
    label?: string;
    error?: string;
    hint?: string;
    required?: boolean;
    class?: string;
    children: Snippet;
  } = $props();

  let id = `field-${crypto.randomUUID().slice(0, 8)}`;
</script>

<div class={cn('flex flex-col gap-1.5', className)}>
  {#if label}
    <label for={id} class="text-sm font-medium">
      {label}
      {#if required}
        <span class="text-destructive">*</span>
      {/if}
    </label>
  {/if}
  {@render children()}
  {#if error}
    <p class="text-sm text-destructive" role="alert">{error}</p>
  {:else if hint}
    <p class="text-sm text-muted-foreground">{hint}</p>
  {/if}
</div>
