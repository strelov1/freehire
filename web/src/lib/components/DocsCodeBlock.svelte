<script lang="ts">
  // A copyable code block for the API docs. Each instance owns its own "copied"
  // flash, so a page full of blocks needs no shared state. Mirrors the copy
  // micro-interaction in CliView, packaged for reuse here.
  let { code, label = 'shell' }: { code: string; label?: string } = $props();

  let copied = $state(false);
  let timer: ReturnType<typeof setTimeout> | undefined;
  async function copy() {
    try {
      await navigator.clipboard.writeText(code);
      copied = true;
      clearTimeout(timer);
      timer = setTimeout(() => (copied = false), 1600);
    } catch {
      // Clipboard can be blocked (no permission / insecure context); the code is
      // visible to select by hand, so a failed copy needs no fallback.
    }
  }
</script>

<figure class="overflow-hidden rounded-lg border border-border bg-secondary/60 font-mono text-sm">
  <figcaption
    class="flex items-center gap-2 border-b border-border px-3 py-1.5 text-xs text-muted-foreground"
  >
    {label}
    <button
      type="button"
      onclick={copy}
      class="ml-auto rounded-md border border-border px-2 py-0.5 text-[11px] font-medium text-muted-foreground transition-colors hover:text-foreground"
    >
      {copied ? 'copied ✓' : 'copy'}
    </button>
  </figcaption>
  <pre class="overflow-x-auto p-3 leading-relaxed">{code}</pre>
</figure>
