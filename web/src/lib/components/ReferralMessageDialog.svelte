<script lang="ts">
  import { resolve } from '$app/paths';
  import { Check, Copy, Loader2 } from '@lucide/svelte';
  import { announce } from '#lib/announce.svelte.js';
  import { trackPlanRefusal } from '#lib/api.js';
  import { Button, Dialog } from '#lib/ui/index.js';

  let { slug, company, onClose }: { slug: string; company: string; onClose: () => void } = $props();
  let open = $state(true);
  let channel = $state<'linkedin' | 'email'>('linkedin');
  let profile = $state('');
  let message = $state('');
  let cited = $state.raw<{ id: string; claim?: string }[]>([]);
  let stage = $state('');
  let drafting = $state(false);
  let error = $state('');
  let upgradeOffered = $state(false);
  let copied = $state(false);
  let controller: AbortController | null = null;

  $effect(() => {
    if (!open) onClose();
  });
  $effect(() => () => controller?.abort());

  function handleFrame(frame: string) {
    const event = /^event: (.+)$/m.exec(frame)?.[1];
    const raw = /^data: (.+)$/m.exec(frame)?.[1];
    if (!event || !raw) return;
    try {
      const data = JSON.parse(raw);
      if (event === 'stage' && !data.done) stage = data.stage;
      if (event === 'message') {
        message = data.body;
        cited = data.cited ?? [];
        announce('Referral message ready');
      }
      if (event === 'stream_error') error = data.error;
    } catch {
      // Ignore an incomplete frame.
    }
  }

  async function generate() {
    if (drafting || !profile.trim() || [...profile].length > 4000) return;
    drafting = true;
    message = '';
    copied = false;
    cited = [];
    error = '';
    upgradeOffered = false;
    stage = 'select';
    controller = new AbortController();
    try {
      const response = await fetch(`/api/v1/jobs/${encodeURIComponent(slug)}/referral-message/stream`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'content-type': 'application/json', accept: 'text/event-stream' },
        body: JSON.stringify({ channel, profile: profile.trim() }),
        signal: controller.signal,
      });
      if (!response.ok || !response.body) {
        const body = await response.json().catch(() => null);
        trackPlanRefusal(response.status, body);
        error = body?.error ?? 'The message could not be drafted.';
        upgradeOffered = response.status === 402 && body?.upgrade_url === '/pricing';
        return;
      }
      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const frames = buffer.split('\n\n');
        buffer = frames.pop() ?? '';
        for (const frame of frames) handleFrame(frame);
      }
      if (!message && !error) error = 'The connection ended before the message was ready. Please try again.';
    } catch (e) {
      if ((e as Error).name !== 'AbortError') error = 'The connection failed. Please try again.';
    } finally {
      drafting = false;
      stage = '';
      controller = null;
      if (error) announce(error, 'assertive');
    }
  }

  async function copy() {
    try {
      await navigator.clipboard.writeText(message);
      copied = true;
      announce('Message copied');
    } catch {
      error = 'Could not copy automatically. Select the text and copy it instead.';
    }
  }
</script>

<Dialog bind:open title={`Draft a message · ${company}`} class="sm:max-w-xl">
  <div class="space-y-4">
    <p class="text-sm text-muted-foreground">
      Paste a few lines from a public profile. We will use them once to draft a message you can edit and send yourself.
      Closing this window clears the text.
    </p>
    <label class="block text-sm font-medium" for="referral-channel">Message type</label>
    <select
      id="referral-channel"
      bind:value={channel}
      disabled={drafting}
      class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
    >
      <option value="linkedin">LinkedIn DM</option>
      <option value="email">Email</option>
    </select>
    <label class="block text-sm font-medium" for="recipient-profile">Contact's public profile text</label>
    <textarea
      id="recipient-profile"
      bind:value={profile}
      disabled={drafting}
      maxlength="4000"
      rows="5"
      placeholder="Name, current role, and a few relevant lines"
      class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
    ></textarea>
    <p class="text-xs text-muted-foreground">The draft uses your job and candidate-stated experience. The contact text is not saved.</p>
    <Button variant="outline" disabled={drafting || !profile.trim()} onclick={() => void generate()}>
      {#if drafting}
        <Loader2 class="size-4 animate-spin" aria-hidden="true" />
        {stage === 'audit' ? 'Checking claims…' : 'Writing…'}
      {:else}
        Draft message
      {/if}
    </Button>
    {#if error}
      <p role="alert" class="text-sm text-destructive">{error}</p>
    {/if}
    {#if upgradeOffered}
      <a href={resolve('/pricing')} class="text-sm text-primary underline">See paid plans</a>
    {/if}
    {#if message}
      <label class="block text-sm font-medium" for="referral-draft">Your message — edit before sending</label>
      <textarea
        id="referral-draft"
        bind:value={message}
        oninput={() => {
          copied = false;
          cited = [];
        }}
        rows="8"
        class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
      ></textarea>
      {#if cited.length}
        <p class="text-xs text-muted-foreground">Experience used in this draft:</p>
        <ul class="list-disc pl-5 text-xs text-muted-foreground">
          {#each cited as item (item.id)}
            <li>{item.claim || 'An achievement no longer in your bank'}</li>
          {/each}
        </ul>
      {/if}
      <Button variant="outline" disabled={!message.trim()} onclick={() => void copy()}>
        {#if copied}
          <Check class="size-4" aria-hidden="true" /> Copied
        {:else}
          <Copy class="size-4" aria-hidden="true" /> Copy message
        {/if}
      </Button>
    {/if}
  </div>
</Dialog>
