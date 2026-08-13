<script lang="ts">
  import { Clock, Mail, MessageSquare } from '@lucide/svelte';
  import { companyLogoUrl } from '$lib/logo';
  import { Badge, EntityLogo } from '$lib/ui';
  import { humanizeStage } from '$lib/stages';
  import { chasedLabel, cvOpenedLabel } from '$lib/followup';
  import type { MyJob } from '$lib/types';

  let {
    item,
    onopen,
  }: {
    item: MyJob;
    onopen: (item: MyJob) => void;
  } = $props();

  const hasNotes = $derived(!!item.notes && item.notes.trim().length > 0);
  // Both readings of a chased application, side by side: the badge says the employer
  // is still quiet, the label says we already prodded them. Neither replaces the other.
  const chased = $derived(chasedLabel(item));
  // A further reading, and like the chase it sits beside the silence badge rather than instead
  // of it: they read the CV and still have not answered.
  const cvOpened = $derived(cvOpenedLabel(item));

  // The posting is gone once cmd/prune removes it, and the card still has to render.
  // The employer and role are carried by the application itself for exactly this.
  const company = $derived(item.job?.company || item.company_slug);
  const title = $derived(item.job?.title || item.role_title);

  // Enter and Space open the application. This runs in the CAPTURE phase, and that is
  // load-bearing rather than a style choice.
  //
  // svelte-dnd-action binds its own keydown to the wrapper element the column puts
  // around each card, and on Enter/Space it starts a keyboard-driven drag and calls
  // stopPropagation. Svelte 5 *delegates* ordinary `onkeydown` to the app root, so a
  // bubble-phase handler here would be reached only after that wrapper listener has
  // already stopped the event — the card would begin a drag and never open. Capture
  // listeners are not delegated, so this one runs first and claims the key.
  //
  // Nothing is lost: the library's guard skips any target whose `disabled` is defined,
  // so while this card was a <button> its keyboard drag never ran either.
  function openOnKey(e: KeyboardEvent) {
    if (e.key !== 'Enter' && e.key !== ' ') return;
    e.preventDefault(); // Space would otherwise scroll the column
    e.stopPropagation();
    onopen(item);
  }
</script>

<!-- The card carries no controls: it is dragged, and it is opened. Every action on the
     application lives in the panel that opens, which has the room for them.

     That is not only a layout preference. svelte-dnd-action refuses to begin a drag
     whose event target carries a `value` property — a guard written for <input>, which
     a <button> also satisfies, its value being "" rather than undefined. This card used
     to mount a button with `after:inset-0` stretched across its whole surface, so every
     mousedown landed on that button and the card could not be picked up at all.

     `role="button"` is an attribute, not an element, so this node has no `value` and the
     guard passes anywhere on the card. The library suppresses the click that ends a real
     drag, so opening and dragging do not collide. -->
<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- The keyboard handler is `onkeydowncapture`, which the rule does not recognise. It
     has to be on the capture phase (see openOnKey), and Enter and Space were both
     confirmed to open the application in a browser. -->
<div
  role="button"
  tabindex="0"
  onclick={() => onopen(item)}
  onkeydowncapture={openOnKey}
  aria-label="Open {title} at {company || 'unknown company'}"
  class="flex w-full cursor-pointer flex-col gap-1.5 rounded-lg border border-border bg-card p-3 text-left shadow-sm transition-colors hover:bg-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
>
  <span class="flex items-center gap-1.5 text-sm font-semibold">
    <EntityLogo name={company} src={companyLogoUrl(company) ?? undefined} shape="square" size="xs" />
    <span class="min-w-0 truncate">{company || 'Unknown company'}</span>
  </span>
  <span class="line-clamp-2 text-sm">{title}</span>
  <!-- Indicators, every one of them. The silence marker used to be the way into the
       follow-up draft; that offer moved into the opened application when the card gave
       up its controls, and the marker went back to reporting. -->
  <span class="flex flex-wrap items-center gap-x-1.5 gap-y-1">
    {#if item.stage}
      <Badge variant="secondary">{humanizeStage(item.stage)}</Badge>
    {/if}
    {#if item.silence_state === 'silent'}
      <span
        class="flex items-center gap-0.5 text-xs tabular-nums text-warning-strong"
        title="No reply for {item.days_silent} days"
        aria-label="No reply for {item.days_silent} days"
      >
        <Clock class="size-3 shrink-0" aria-hidden="true" />
        {item.days_silent}d
      </span>
    {:else if item.silence_state === 'unconfirmed'}
      <span
        class="flex items-center gap-0.5 text-xs text-muted-foreground"
        title="Mail may be from them — confirm the link to know"
        aria-label="Mail awaiting confirmation"
      >
        <MessageSquare class="size-3 shrink-0" aria-hidden="true" />
        ?
      </span>
    {/if}
    {#if chased}
      <span class="text-xs text-muted-foreground">{chased}</span>
    {/if}
    {#if cvOpened}
      <span
        class="text-xs text-muted-foreground"
        title="A link in the CV you sent was opened. Company mail scanners follow links automatically, so this is a hint rather than proof someone read it."
      >{cvOpened}</span>
    {/if}
    {#if item.email_count > 0}
      <span
        class="flex items-center gap-0.5 text-xs tabular-nums text-muted-foreground"
        title="{item.email_count} linked email{item.email_count === 1 ? '' : 's'}"
        aria-label="{item.email_count} linked email{item.email_count === 1 ? '' : 's'}"
      >
        <Mail class="size-3 shrink-0" aria-hidden="true" />
        {item.email_count}
      </span>
    {/if}
    {#if hasNotes}
      <svg
        class="size-3 shrink-0 text-muted-foreground"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        role="img"
        aria-label="Has notes"
      >
        <title>Has notes</title>
        <path d="M8 7h8M8 12h8M8 17h5" />
      </svg>
    {/if}
  </span>
</div>
