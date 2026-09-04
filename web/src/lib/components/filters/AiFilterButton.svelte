<script lang="ts">
  import { MessageSquareText } from '@lucide/svelte';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { promptSignIn } from '$lib/signin';
  import type { FilterStore } from '$lib/filters';
  import AiFilterDialog from './AiFilterDialog.svelte';

  // Rendered for everyone, signed in or not. Hiding it from signed-out visitors would
  // hide the feature from exactly the people who have not yet been given a reason to
  // sign in; the gate is on activation, which sends them to /signin.
  let { store }: { store: FilterStore } = $props();

  let open = $state(false);

  function activate() {
    if (!isAuthenticated()) {
      promptSignIn();
      return;
    }
    open = true;
  }
</script>

<button
  type="button"
  class="flex w-full items-center justify-center gap-2 rounded-xl border border-primary/30 bg-primary/5 px-3 py-2 text-sm font-medium text-primary transition-colors hover:bg-primary/10"
  onclick={activate}
>
  <MessageSquareText class="size-4" />
  Describe with AI
  <span class="rounded bg-primary/15 px-1 text-xs font-semibold uppercase tracking-wide">beta</span>
</button>

{#if open}
  <AiFilterDialog {store} onclose={() => (open = false)} />
{/if}
