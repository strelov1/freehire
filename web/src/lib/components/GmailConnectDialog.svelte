<script lang="ts">
  import { Mail, Lock, Search, Link, Tags, TrendingUp, ArrowRight, Check } from '@lucide/svelte';
  import { Button, Dialog } from '#lib/ui/index.js';

  // The parent owns open/close; onConnect proceeds to the Google OAuth redirect.
  let { onClose, onConnect }: { onClose: () => void; onConnect: () => void } = $props();

  // The parent mounts this component to open it and unmounts on onClose, so the
  // dialog is open for its whole life. Dialog owns the closing — Escape, the
  // backdrop and its own button all land on `open`, and this reports it upward
  // rather than each affordance calling onClose itself.
  let open = $state(true);
  $effect(() => {
    if (!open) onClose();
  });

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
</script>

<Dialog
  bind:open
  title="Connect Gmail"
  description="Turn recruiting emails into application updates, without sorting them by hand."
  class="sm:max-w-xl"
>
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
    <figcaption class="mb-3 text-xs font-medium text-muted-foreground">For example: an interview invitation</figcaption>
    <div class="flex flex-wrap items-center gap-3 text-sm">
      <span class="inline-flex items-center gap-2">
        <Mail class="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
        “Let’s meet”
      </span>
      <ArrowRight class="size-4 text-muted-foreground" aria-hidden="true" />
      <span class="inline-flex items-center gap-1.5 rounded-md bg-brand-muted px-2 py-1 font-medium text-brand-strong">
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
        Google grants read access to Gmail; freehire uses job-related searches to find relevant mail. It cannot send,
        edit or delete messages. Your connection token is stored encrypted.
      </p>
    </div>
  </div>

  <div class="mt-5 flex justify-end gap-2">
    <Button type="button" variant="outline" onclick={() => (open = false)}>Cancel</Button>
    <Button variant="primary" onclick={onConnect} class="gap-1.5">Connect Gmail <Mail class="size-4" /></Button>
  </div>
</Dialog>
