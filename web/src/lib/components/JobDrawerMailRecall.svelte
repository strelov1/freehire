<script lang="ts">
  import { api, isPlanRefusal } from '#lib/api.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, plural, t } from '#lib/i18n/t.js';
  import type { MailRecallResult, RecalledEmail } from '#lib/types.js';
  import { Button } from '#lib/ui/index.js';
  import { errorMessage, timeAgo } from '#lib/utils.js';
  import PlanLimitLink from './PlanLimitLink.svelte';
  import { messages } from './JobDrawer.messages';

  // The Emails tab's mailbox sweep for one application (`slug`), shown by JobDrawer only
  // when the application can be swept: `canRecall` gates the button. `company` names the
  // employer in the notice; `onlinked` reloads the tab's mail once a message is linked.
  let {
    slug,
    company,
    canRecall,
    onlinked,
  }: { slug: string; company: string; canRecall: boolean; onlinked: () => Promise<void> } = $props();

  const s = $derived(t(messages, locale()));

  // The mailbox sweep: from this application, find the mail that belongs to it. What comes
  // back are SUGGESTIONS — nothing is linked — resolved by the same confirm/reject calls
  // the inbox uses. Null until the button is pressed, so an untouched tab shows no verdict.
  let recall = $state.raw<MailRecallResult | null>(null);
  let recallLoading = $state(false);
  let recallError = $state<string | null>(null);
  let recallPlanRefused = $state(false);
  // Ids resolved since the sweep, so a confirmed or dismissed row leaves at the press
  // rather than on the next load.
  let recallResolved = $state.raw<string[]>([]);
  const recallPending = $derived(
    (recall?.suggested ?? []).filter((e) => !recallResolved.includes(e.provider_id ?? String(e.id))),
  );

  async function runRecall() {
    if (recallLoading) return;
    recallLoading = true;
    recallError = null;
    recallPlanRefused = false;
    recallResolved = [];
    try {
      recall = await api.recallApplicationMail(slug);
    } catch (e) {
      // Shown rather than swallowed. An empty result would read as "your mailbox holds
      // nothing", which is the wrong thing to say about a gateway being down.
      recall = null;
      recallPlanRefused = isPlanRefusal(e);
      recallError = recallPlanRefused ? s.errors.searchMailNeedsPlan : errorMessage(e, s.errors.searchMail);
    } finally {
      recallLoading = false;
    }
  }

  // A proposal arrives one of two ways and leaves the same way. From the mailbox search it
  // is a provider id and nothing of ours yet, so linking imports it first; from stored mail
  // it is a row carrying a suggestion, resolved by the calls the inbox already uses.
  async function resolveRecalled(e: RecalledEmail, accept: boolean) {
    const key = e.provider_id ?? String(e.id);
    recallResolved = [...recallResolved, key];
    try {
      if (e.provider_id) {
        if (accept) await api.linkRecalledMail(slug, e.provider_id);
      } else {
        await (accept ? api.confirmEmailLink(e.id) : api.rejectEmailLink(e.id));
      }
      if (accept) await onlinked();
    } catch {
      // Put it back: a proposal the caller can press again beats a row that vanished
      // without becoming anything.
      recallResolved = recallResolved.filter((x) => x !== key);
    }
  }
</script>

<!-- The pull direction. Everything else in the mail stack starts from a message
     and asks which application it belongs to; this asks the opposite, which is
     the only way an application that plainly ought to have mail can say so. -->
{#if canRecall}
  <div class="flex flex-wrap items-center gap-2">
    <Button size="sm" variant="outline" disabled={recallLoading} onclick={runRecall}>
      {recallLoading ? s.searchingMail : s.findMail}
    </Button>
    {#if recall && recallPending.length === 0}
      <span class="text-xs text-muted-foreground">
        {recall.scanned === 0
          ? s.noUnattachedMail
          : format(plural(locale(), recall.scanned, s.noMatchAmong), { scanned: String(recall.scanned) })}
      </span>
    {/if}
  </div>
{/if}
{#if recallPlanRefused}
  <div class="flex flex-wrap items-center gap-3">
    <p class="text-sm text-muted-foreground">{recallError}</p>
    <PlanLimitLink />
  </div>
{:else if recallError}
  <p class="text-sm text-destructive">{recallError}</p>
{/if}
{#if recallPending.length > 0}
  <div class="flex flex-col gap-2 rounded-xl border border-dashed border-border p-3">
    <p class="text-sm">
      {format(plural(locale(), recall?.scanned ?? 0, s.possibleMatches), {
        count: String(recallPending.length),
        scanned: String(recall?.scanned ?? 0),
      })}
    </p>
    <!-- Said where the boundary is crossed, not buried in a settings page: the
         sweep looks through the mailbox and keeps nothing until Link is pressed. -->
    <p class="text-xs text-muted-foreground">
      {format(s.searchedMailboxFor, { company })}
    </p>
    {#each recallPending as e (e.provider_id ?? e.id)}
      <div class="flex flex-wrap items-center gap-2 rounded-md border border-border px-3 py-2">
        <div class="min-w-0 flex-1">
          <div class="flex items-baseline gap-2">
            <span class="min-w-0 flex-1 truncate text-sm font-medium">{e.from_name || e.from_addr}</span>
            <span class="shrink-0 text-xs text-muted-foreground">{timeAgo(e.received_at, locale())}</span>
          </div>
          <div class="truncate text-sm text-muted-foreground">{e.subject || s.noSubject}</div>
          <!-- Marked on the row it belongs to, not only counted below it. The
               count alone made the reader hunt for which message it meant. -->
          {#if e.invitation}
            <span class="mt-1 inline-flex text-xs text-muted-foreground">
              {s.carriesInvitation}
            </span>
          {/if}
        </div>
        <Button size="sm" class="shrink-0" onclick={() => resolveRecalled(e, true)}>{s.link}</Button>
        <button
          type="button"
          class="shrink-0 text-xs text-muted-foreground underline-offset-2 hover:underline"
          onclick={() => resolveRecalled(e, false)}
        >
          {s.notThisOne}
        </button>
      </div>
    {/each}
    <!-- The calendar follows from the mail and needs no calendar code: cal-sync
         re-reads its whole window every run, so an invitation linked today
         produces its meeting on the next one. -->
    {#if recall && recall.invitations > 0}
      <p class="text-xs text-muted-foreground">
        {s.invitationSyncNote}
      </p>
    {/if}
  </div>
{/if}
