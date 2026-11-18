<script lang="ts">
  import { resolve } from '$app/paths';
  import { CheckCircle2 } from '@lucide/svelte';
  import { Button } from '#lib/ui/index.js';

  // The job page's one slot for what follows an Apply: the "did you apply?" question
  // (`askApplied`, answered through `onconfirm`/`ondismiss`), the confirmation once it is on
  // the board (`justApplied`), an auto-apply that is being prepared (`queued`, tracked under
  // `slug`) and an auto-apply that could not start (`error`).
  let {
    askApplied,
    justApplied,
    queued,
    slug,
    error,
    onconfirm,
    ondismiss,
  }: {
    askApplied: boolean;
    justApplied: boolean;
    queued: boolean;
    slug: string;
    error: string | null;
    onconfirm: () => void;
    ondismiss: () => void;
  } = $props();
</script>

<!-- The three states of one exchange. They sit at the top of the content column rather
     than beside the CTA that raised them: the question follows a click that opened a
     NEW TAB, so the reader meets it on returning to this one, and the first thing under
     the tabs is what a returning eye lands on. It is also the widest slot on the page,
     which a two-button question needs and the title row — already carrying the CTAs —
     does not have. They share one slot so that answering "Yes, save" replaces the
     question in place; split across the page, the confirmation would read as a second,
     unrelated banner appearing somewhere the reader was not looking. -->
{#if askApplied}
  <div class="flex flex-wrap items-center justify-between gap-3 rounded-md border border-border bg-secondary px-4 py-3">
    <span class="text-sm">Did you apply to this job?</span>
    <div class="flex items-center gap-2">
      <Button variant="primary" size="sm" onclick={onconfirm}>Yes, save</Button>
      <Button variant="ghost" size="sm" onclick={ondismiss}>No</Button>
    </div>
  </div>
{/if}

{#if justApplied}
  <div
    class="flex flex-wrap items-center justify-between gap-3 rounded-md border border-brand/30 bg-brand-muted px-4 py-3"
  >
    <span class="inline-flex items-center gap-1.5 text-sm font-medium text-brand-strong">
      <CheckCircle2 class="size-4 shrink-0" aria-hidden="true" /> Added to your board
    </span>
    <a href={resolve('/my/tracking')} class="text-sm font-medium text-brand-strong underline underline-offset-4">
      View on your board →
    </a>
  </div>
{/if}

{#if queued}
  <div
    class="flex flex-wrap items-center justify-between gap-3 rounded-md border border-brand/30 bg-brand-muted px-4 py-3"
  >
    <span class="inline-flex items-center gap-1.5 text-sm font-medium text-brand-strong">
      <CheckCircle2 class="size-4 shrink-0" aria-hidden="true" /> We're preparing a tailored CV — you'll get a notification
      to review it.
    </span>
    <a
      href={resolve('/my/tracking/[id]', { id: slug })}
      class="text-sm font-medium text-brand-strong underline underline-offset-4"
    >
      Track progress →
    </a>
  </div>
{/if}
{#if error}
  <p role="alert" class="text-sm text-destructive">{error}</p>
{/if}
