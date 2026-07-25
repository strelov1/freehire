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
    /** Receives the ids to spread onto the control: `{@render children({ id, describedBy })}`. */
    children: Snippet<[{ id: string; describedBy: string | undefined }]>;
  } = $props();

  // $props.id() is stable across SSR and hydration — crypto.randomUUID() is not.
  const uid = $props.id();
  const id = `${uid}-control`;
  const messageId = `${uid}-message`;
  let describedBy = $derived(error || hint ? messageId : undefined);
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
  {@render children({ id, describedBy })}
  {#if error}
    <p id={messageId} class="text-sm text-destructive" role="alert">{error}</p>
  {:else if hint}
    <p id={messageId} class="text-sm text-muted-foreground">{hint}</p>
  {/if}
</div>
