<script lang="ts">
  import { Mail, X, Lock, Search, Link, Tags, TrendingUp, ArrowRight, Check } from '@lucide/svelte';
  import { Button, lockScroll, unlockScroll } from '#lib/ui/index.js';
  import { focusTrap } from '#lib/actions/focusTrap.js';

  // The parent owns open/close; onConnect proceeds to the Google OAuth redirect.
  let { onClose, onConnect }: { onClose: () => void; onConnect: () => void } = $props();

  const steps = [
    {
      title: 'Find job-related mail',
      body: 'Searches for recruiting messages and replies about your applications, so you spend less time digging through Gmail.',
      icon: Search,
    },
    {
      title: 'Connect it to your application',
      body: 'Uses the company in the sender or subject to match a reply to an application you track, keeping the conversation together.',
      icon: Link,
    },
    {
      title: 'Recognize the update',
      body: 'AI reads the message and labels it — from an application received to an interview, offer or rejection — so you can see what changed.',
      icon: Tags,
    },
    {
      title: 'Keep your pipeline current',
      body: 'Files the email under the application. A confident interview or offer update moves its stage forward, never backward.',
      icon: TrendingUp,
    },
  ];

  // Mounted only while open, so the lock lives exactly as long as the dialog does.
  $effect(() => {
    lockScroll();
    return unlockScroll;
  });
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && onClose()} />

<div class="fixed inset-0 z-50 flex items-center justify-center p-4">
  <button
    type="button"
    aria-label="Close dialog"
    class="absolute inset-0 animate-modal-fade bg-black/50"
    onclick={onClose}
  ></button>

  <div
    role="dialog"
    aria-modal="true"
    aria-labelledby="gmail-connect-title"
    aria-describedby="gmail-connect-description"
    class="relative z-10 flex animate-modal-pop max-h-[85vh] w-full max-w-xl flex-col overflow-hidden rounded-2xl border border-border bg-background shadow-xl"
    {@attach focusTrap()}
  >
    <div class="flex items-start gap-3 border-b border-border p-5">
      <div class="flex size-10 shrink-0 items-center justify-center rounded-xl bg-brand-muted">
        <Mail class="size-5 text-brand-strong" />
      </div>
      <div class="min-w-0 flex-1">
        <h2 id="gmail-connect-title" class="text-lg font-semibold leading-tight">Connect Gmail</h2>
        <p id="gmail-connect-description" class="mt-1 text-sm text-muted-foreground">
          Turn recruiting emails into application updates, without sorting them by hand.
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

    <div class="flex-1 overflow-y-auto overscroll-contain p-5">
      <ol>
        {#each steps as step, i (step.title)}
          <li class="relative flex gap-4 pb-6 last:pb-0">
            {#if i < steps.length - 1}
              <span aria-hidden="true" class="absolute bottom-0 left-5 top-10 w-px bg-border"></span>
            {/if}
            <span
              class="relative flex size-10 shrink-0 items-center justify-center rounded-full bg-brand-muted text-brand-strong"
              aria-hidden="true"
            >
              <step.icon class="size-4.5" />
            </span>
            <div class="min-w-0 pt-0.5">
              <h3 class="text-sm font-semibold">{step.title}</h3>
              <p class="mt-1 text-sm leading-relaxed text-muted-foreground">{step.body}</p>
            </div>
          </li>
        {/each}
      </ol>

      <figure class="mt-6 rounded-xl border border-border p-4">
        <figcaption class="mb-3 text-xs font-medium text-muted-foreground">
          For example: an interview invitation
        </figcaption>
        <div class="flex flex-wrap items-center gap-3 text-sm">
          <span class="inline-flex items-center gap-2">
            <Mail class="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
            “Let’s meet”
          </span>
          <ArrowRight class="size-4 text-muted-foreground" aria-hidden="true" />
          <span
            class="inline-flex items-center gap-1.5 rounded-md bg-brand-muted px-2 py-1 font-medium text-brand-strong"
          >
            <Check class="size-3.5" aria-hidden="true" /> Interview
          </span>
        </div>
        <p class="mt-2 text-xs leading-relaxed text-muted-foreground">
          The reply and its status live with the application, so your next step is easy to find.
        </p>
      </figure>

      <div class="mt-5 flex items-start gap-3 text-xs leading-relaxed text-muted-foreground">
        <Lock class="mt-0.5 size-4 shrink-0" aria-hidden="true" />
        <div>
          <p class="font-medium text-foreground">Read-only access. You stay in control.</p>
          <p class="mt-1">
            Google grants read access to Gmail; freehire uses job-related searches to find relevant mail. It cannot
            send, edit or delete messages. Your connection token is stored encrypted.
          </p>
        </div>
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
