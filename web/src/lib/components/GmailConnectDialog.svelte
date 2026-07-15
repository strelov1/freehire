<script lang="ts">
  import { Mail, X, Lock, ExternalLink } from '@lucide/svelte';
  import { Button } from '$lib/ui';

  // The parent owns open/close; onConnect proceeds to the Google OAuth redirect.
  let { onClose, onConnect }: { onClose: () => void; onConnect: () => void } = $props();

  const REPO = 'https://github.com/strelov1/freehire/blob/main';
  // The pipeline stages, each linking to the exact source that implements it — the
  // project is open source, so "how it works" is verifiable, not a promise.
  const steps: { title: string; body: string; href: string }[] = [
    {
      title: 'Read',
      body: 'A Gmail search scoped to recruiting/ATS platforms only — never your personal mail. Read-only access; the token is encrypted at rest.',
      href: `${REPO}/internal/gmailsync/senders.go`,
    },
    {
      title: 'Match',
      body: 'Each email is linked to one of your tracked applications by the company name in the sender/subject — deterministic, and it never guesses.',
      href: `${REPO}/internal/mailmatch`,
    },
    {
      title: 'Classify',
      body: 'An LLM reads the body and tags a status: acknowledgement · screening · interview · assessment · offer · rejection.',
      href: `${REPO}/internal/mailclassify`,
    },
    {
      title: 'Sort',
      body: 'The email appears under its application with a status badge; a confident interview/offer advances your pipeline stage (never backward).',
      href: `${REPO}/cmd/classify-mail`,
    },
  ];
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && onClose()} />

<div class="fixed inset-0 z-50 flex items-center justify-center p-4">
  <button type="button" aria-label="Close dialog" class="absolute inset-0 bg-black/50" onclick={onClose}></button>

  <div
    role="dialog"
    aria-modal="true"
    aria-label="Connect Gmail"
    class="relative z-10 flex max-h-[85vh] w-full max-w-lg flex-col overflow-hidden rounded-2xl border border-border bg-background shadow-xl"
  >
    <div class="flex items-start gap-3 border-b border-border p-5">
      <div class="flex size-10 shrink-0 items-center justify-center rounded-xl bg-brand-muted">
        <Mail class="size-5 text-brand-strong" />
      </div>
      <div class="min-w-0 flex-1">
        <h2 class="text-lg font-semibold leading-tight">Connect Gmail</h2>
        <p class="mt-0.5 text-sm text-muted-foreground">
          freehire reads only your recruiting mail and sorts it under your applications — automatically.
        </p>
      </div>
      <button
        type="button"
        onclick={onClose}
        aria-label="Close"
        class="-mr-1 rounded-full p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
      >
        <X class="size-5" />
      </button>
    </div>

    <div class="flex-1 overflow-y-auto p-5">
      <ol class="flex flex-col gap-4">
        {#each steps as step, i (step.title)}
          <li class="flex gap-3">
            <span
              class="mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-semibold text-muted-foreground"
            >
              {i + 1}
            </span>
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <span class="text-sm font-medium">{step.title}</span>
                <a
                  href={step.href}
                  target="_blank"
                  rel="noopener noreferrer"
                  class="inline-flex items-center gap-0.5 text-xs text-brand-strong hover:underline"
                >
                  source <ExternalLink class="size-3" />
                </a>
              </div>
              <p class="mt-0.5 text-sm text-muted-foreground">{step.body}</p>
            </div>
          </li>
        {/each}
      </ol>

      <div class="mt-5 flex items-start gap-2 rounded-lg border border-border bg-muted/40 p-3 text-xs text-muted-foreground">
        <Lock class="mt-0.5 size-3.5 shrink-0" />
        <span>
          Read-only Gmail scope, only mail from recruiting platforms is fetched, and it's all
          <a
            href="https://github.com/strelov1/freehire"
            target="_blank"
            rel="noopener noreferrer"
            class="font-medium text-brand-strong hover:underline">open source</a
          > — you can read exactly what runs.
        </span>
      </div>
    </div>

    <div class="flex justify-end gap-2 border-t border-border p-4">
      <Button variant="outline" onclick={onClose}>Cancel</Button>
      <Button variant="primary" onclick={onConnect} class="gap-1.5">
        Connect Gmail <Mail class="size-4" />
      </Button>
    </div>
  </div>
</div>
