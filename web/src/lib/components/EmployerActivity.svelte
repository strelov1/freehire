<script lang="ts">
  import { untrack } from 'svelte';
  import { History } from '@lucide/svelte';
  import { api } from '#lib/api.js';
  import type { components } from '#lib/generated/api.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { defineMessages, format, t } from '#lib/i18n/t.js';
  import type { EmployerApplicationStage } from '#lib/types.js';
  import { Button } from '#lib/ui/index.js';
  import { formatDate } from '#lib/utils.js';
  import { messages as applicationMessages } from './EmployerApplications.messages';
  import States from './States.svelte';

  const messages = defineMessages(
    {
      title: 'Activity',
      empty: 'No activity recorded yet.',
      unavailable: 'Could not load activity.',
      retry: 'Try again',
      more: 'Load earlier activity',
      unknown: 'Former or unavailable team member',
      stage_changed: 'Application #{id} moved from {previous} to {stage}',
      notes_changed: 'Private notes updated for application #{id}',
      invited: 'Invitation created for {email}',
      invitation_withdrawn: 'Invitation withdrawn for {email}',
      member_joined: '{email} joined the team',
      member_removed: '{email} left the team',
    },
    {
      ru: {
        title: 'История действий',
        empty: 'Записей пока нет.',
        unavailable: 'Не удалось загрузить историю.',
        retry: 'Попробовать снова',
        more: 'Загрузить более ранние действия',
        unknown: 'Участник команды недоступен',
        stage_changed: 'Стадия отклика №{id}: {previous} → {stage}',
        notes_changed: 'Обновлены приватные заметки к отклику №{id}',
        invited: 'Создано приглашение для {email}',
        invitation_withdrawn: 'Приглашение для {email} отозвано',
        member_joined: '{email} вступил в команду',
        member_removed: '{email} выбыл из команды',
      },
    },
  );

  type Event = components['schemas']['EmployerActivity'];
  const s = $derived(t(messages, locale()));
  const stages = $derived(t(applicationMessages, locale()).stages);

  let events = $state<Event[]>([]);
  let loading = $state(false);
  let failed = $state(false);
  let more = $state(false);

  $effect(() => {
    untrack(() => void load());
  });

  async function load(before?: number) {
    if (loading) return;
    loading = true;
    failed = false;
    try {
      const next = await api.listEmployerActivity(before);
      events = before ? [...events, ...next] : next;
      more = next.length === 30;
    } catch {
      failed = true;
    } finally {
      loading = false;
    }
  }

  function stage(value?: string) {
    return (value && stages[value as EmployerApplicationStage]) || value || '';
  }

  function describe(event: Event) {
    const { action } = event;
    switch (action) {
      case 'stage_changed':
        return format(s[action], {
          id: String(event.application_id),
          previous: stage(event.previous_stage),
          stage: stage(event.stage),
        });
      case 'notes_changed':
        return format(s[action], { id: String(event.application_id) });
      case 'invited':
      case 'invitation_withdrawn':
      case 'member_joined':
      case 'member_removed':
        return format(s[action], { email: event.subject_email ?? '' });
      default:
        return action;
    }
  }
</script>

<section class="flex max-w-xl flex-col gap-3 border-t border-border pt-6">
  <h2 class="flex items-center gap-1.5 text-sm font-semibold">
    <History class="size-4 text-muted-foreground" />
    {s.title}
  </h2>
  {#if loading && !events.length}
    <States state="loading" rows={2} />
  {:else if !failed && !events.length}
    <p class="text-sm text-muted-foreground">{s.empty}</p>
  {/if}
  {#if events.length}
    <ul class="divide-y divide-border rounded-lg border border-border">
      {#each events as event (event.id)}
        <li class="flex flex-col gap-0.5 px-4 py-2.5">
          <p class="break-words text-sm">{describe(event)}</p>
          <p class="break-words text-xs text-muted-foreground">
            {event.actor_email || s.unknown} · {formatDate(event.occurred_at, locale())}
          </p>
        </li>
      {/each}
    </ul>
  {/if}
  {#if failed}
    <p class="text-sm text-destructive">{s.unavailable}</p>
  {/if}
  {#if more || failed}
    <Button variant="outline" size="sm" class="w-fit" disabled={loading} onclick={() => load(events.at(-1)?.id)}>
      {failed ? s.retry : s.more}
    </Button>
  {/if}
</section>
