<script lang="ts">
  import { resolve } from '$app/paths';
  import { Dialog } from '$lib/ui';

  // Points a candidate's own coding agent at this CV, without teaching the reader
  // the CLI's flags — an agent with the freehire-tailor-cv skill installed (ships
  // in freehire-cli, see /cli) already knows the cv context/get/edit/render loop
  // and the evidence rule; all it's missing is which CV. So the copyable text is
  // one prompt naming this CV's id, not the commands themselves.
  let { cvId, onClose }: { cvId: string; onClose: () => void } = $props();

  let open = $state(true);
  $effect(() => {
    if (!open) onClose();
  });

  const prompt = $derived(`Tailor my freehire CV (id ${cvId}) to this vacancy using the freehire CLI.`);

  let copied = $state(false);
  let copyTimer: ReturnType<typeof setTimeout> | undefined;
  async function copyPrompt() {
    try {
      await navigator.clipboard.writeText(prompt);
      copied = true;
      clearTimeout(copyTimer);
      copyTimer = setTimeout(() => (copied = false), 1600);
    } catch {
      // Clipboard can be blocked (no permission / insecure context) — the prompt
      // is plainly visible to select by hand, so a failed copy needs no fallback.
    }
  }
</script>

<Dialog bind:open title="Edit this CV from the CLI" class="sm:max-w-lg">
  <p class="text-sm leading-relaxed text-muted-foreground">
    Give this to your own coding agent — with the freehire CLI installed and signed in, it reads
    and writes this exact CV the same way the in-app assistant does.
  </p>
  <div class="mt-3 flex items-start gap-2 rounded-md border border-border bg-background/60 p-3 font-mono text-sm leading-relaxed">
    <p class="min-w-0 flex-1 break-words">{prompt}</p>
    <button
      type="button"
      onclick={copyPrompt}
      class="shrink-0 rounded-md border border-border px-2 py-0.5 font-sans text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
    >
      {copied ? 'copied ✓' : 'copy'}
    </button>
  </div>
  <p class="mt-3 text-sm leading-relaxed text-muted-foreground">
    Needs your own
    <a
      href={resolve('/my/api-keys')}
      class="font-medium text-foreground underline-offset-4 hover:underline">API key</a
    > and the CLI installed and signed in — setup and the rest of the commands are on the
    <a href={resolve('/cli')} class="font-medium text-foreground underline-offset-4 hover:underline"
      >CLI reference</a
    >.
  </p>
</Dialog>
