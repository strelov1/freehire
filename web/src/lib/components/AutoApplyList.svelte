<script lang="ts">
  import { resolve } from '$app/paths';
  import { ArrowRight, RefreshCw } from '@lucide/svelte';
  import { api } from '#lib/api.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import type { AutoApplyListItem } from '#lib/types.js';
  import { Badge, Button } from '#lib/ui/index.js';
  import { messages } from './activity.messages.js';
  import States from './States.svelte';

  const s = $derived(t(messages, locale()).autoApply);
  let status = $state<'loading' | 'error' | 'ready'>('loading');
  let items = $state<AutoApplyListItem[]>([]);
  let revision = $state(0);

  $effect(() => {
    if (!isAuthenticated()) return;
    void revision;
    let cancelled = false;
    status = 'loading';
    api.listAutoApply().then(
      (result) => {
        if (cancelled) return;
        items = result.attempts;
        status = 'ready';
      },
      () => {
        if (!cancelled) status = 'error';
      },
    );
    return () => {
      cancelled = true;
    };
  });

  function refresh() {
    revision += 1;
  }
</script>

<div class="flex flex-col gap-4">
  <div class="flex items-start justify-between gap-4">
    <p class="max-w-prose text-sm text-muted-foreground">{s.description}</p>
    <Button variant="outline" size="sm" disabled={status === 'loading'} onclick={refresh}>
      <RefreshCw class="size-3.5" />{s.refresh}
    </Button>
  </div>

  {#if status === 'loading'}
    <States state="loading" />
  {:else if status === 'error'}
    <div role="alert" class="flex flex-col items-center gap-3 py-8">
      <p class="text-sm text-destructive">{s.loadError}</p>
      <Button variant="outline" size="sm" onclick={refresh}>{s.retry}</Button>
    </div>
  {:else if items.length === 0}
    <States state="empty" message={s.empty} />
  {:else}
    <ul class="divide-y divide-border">
      {#each items as item (item.job_slug)}
        <li>
          <a
            href={resolve('/my/tracking/[id]?tab=auto_apply', { id: item.job_slug })}
            class="group flex items-center gap-3 rounded-md px-2 py-4 transition-colors hover:bg-accent/40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
          >
            <div class="flex min-w-0 flex-1 flex-col gap-2">
              <div class="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
                <div class="min-w-0 flex-1 basis-48">
                  <span class="block break-words font-medium">{item.title}</span>
                  <span class="text-sm text-muted-foreground">{item.company || s.unknownCompany}</span>
                </div>
                <Badge variant={item.status === 'submitted' ? 'brand' : 'outline'}>
                  {s.statuses[item.status]}
                </Badge>
              </div>
              <p class="text-sm text-muted-foreground">{s.details[item.status]}</p>
              <time datetime={item.last_activity_at} class="text-xs tabular-nums text-muted-foreground">
                {new Date(item.last_activity_at).toLocaleString(locale(), { dateStyle: 'medium', timeStyle: 'short' })}
              </time>
            </div>
            <ArrowRight class="size-4 shrink-0 text-muted-foreground" />
          </a>
        </li>
      {/each}
    </ul>
  {/if}
</div>
