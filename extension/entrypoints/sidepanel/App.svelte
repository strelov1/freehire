<script lang="ts">
  import { onMount } from 'svelte';
  import { browser } from 'wxt/browser';
  import { type RuntimeMessage, type PageSnapshot } from '../../lib/protocol';
  import { createSession, getSession, SessionNotFound } from '../../lib/assistant/api';
  import { sendTurn, type Turn } from '../../lib/assistant/client';
  import { initChat, reduceTurnEvent, type ChatState } from '../../lib/assistant/chat';
  import { eventsFromTranscript } from '../../lib/assistant/wire';
  import { recallSession, rememberSession, forgetSession } from '../../lib/assistant/session';
  import { signIn, signOut, getToken, fetchMe, type HireUser } from '../../lib/auth';
  import {
    freehireSlugFromUrl,
    findJob,
    getJob,
    getMatch,
    getMatchText,
    getAutofillProfile,
    runAgentAutofill,
    resolveJob,
    resolveNotice,
    type FreehireJob,
    type JobMatch,
    type AutofillProfile,
  } from '../../lib/freehire';
  import { planLabelFills, looksLikeApplication, scopeToApplication } from '../../lib/form';
  import { ToolChannel } from '../../lib/tools/client';
  import { activeTabPage } from '../../lib/tools/page';
  import MatchCard from './MatchCard.svelte';
  import ToolGroupList from './ToolGroupList.svelte';
  import JobDeck from './JobDeck.svelte';
  import { splitPresentingCalls } from '../../lib/assistant/deck';
  import { Alert, Badge, Button, EmptyState, Input, Skeleton } from 'freehire-design-system';

  let chat = $state<ChatState>(initChat());
  // Local action feedback (autofill results, errors) — not part of a turn.
  let notices = $state<string[]>([]);
  let draft = $state('');
  let sending = $state(false);
  let chatError = $state('');
  let restoring = $state(false);

  // The conversation this panel is holding, and the turn in flight if there is
  // one. Plain refs: neither is rendered directly.
  let sessionId: string | null = null;
  let turn: Turn | null = null;

  let user = $state<HireUser | null>(null);
  let authBusy = $state(false);
  let authError = $state('');

  // The browser-tool wire: while the panel is open this holds the socket the
  // agent drives this browser through. It lives here rather than in the service
  // worker because only the panel stays alive.
  const tools = new ToolChannel(activeTabPage);

  type MatchStatus = 'idle' | 'loading' | 'ready' | 'error' | 'empty';
  let matchStatus = $state<MatchStatus>('idle');
  let matchJob = $state<FreehireJob | null>(null);
  let match = $state<JobMatch | null>(null);
  let matchError = $state('');

  onMount(() => {
    // The conversation is created lazily on the first message, so an idle panel
    // starts nothing; a conversation held earlier is repainted here.
    void restoreSession();

    // Re-run the match when the user switches tabs or a page finishes loading,
    // so the card tracks whatever job page is in front — like the reference.
    const refresh = () => {
      if (user) void loadMatch();
    };
    browser.tabs.onActivated.addListener(refresh);
    const onUpdated = (_id: number, info: { status?: string }) => {
      if (info.status === 'complete') refresh();
    };
    browser.tabs.onUpdated.addListener(onUpdated);

    return () => {
      turn?.cancel();
      tools.stop();
      browser.tabs.onActivated.removeListener(refresh);
      browser.tabs.onUpdated.removeListener(onUpdated);
    };
  });

  async function restoreSession() {
    const token = await getToken();
    if (!token) return;
    user = await fetchMe(token);
    if (!user) return;
    tools.start(token);
    void loadMatch();
    void restoreConversation(token);
  }

  /**
   * Repaint the conversation this panel was holding. The transcript is replayed
   * through the same reducer a live turn folds through, so history and a running
   * turn cannot render differently.
   *
   * A conversation the server no longer has (deleted from the web) is not an error
   * the user can act on from here — forget it and let the next message start a
   * fresh one.
   */
  async function restoreConversation(token: string) {
    const remembered = await recallSession();
    if (!remembered) return;
    restoring = true;
    try {
      const { messages } = await getSession(remembered, token);
      // The composer unlocks as soon as the user is known, so a message can be
      // sent while this read is in flight — and that message created its own
      // conversation. Adopting the remembered one now would point the panel at A
      // while storage holds B, and lose the exchange the user just watched.
      if (sessionId) return;
      sessionId = remembered;
      for (const event of eventsFromTranscript(messages)) {
        chat = reduceTurnEvent(chat, event);
      }
    } catch (err) {
      if (err instanceof SessionNotFound) {
        await forgetSession();
      } else {
        chatError = `Could not load your conversation: ${err instanceof Error ? err.message : 'error'}`;
      }
    } finally {
      restoring = false;
    }
  }

  // Bumped at the start of every loadMatch() call; a stale call checks its own
  // captured id against the current one before writing shared state, so an
  // earlier in-flight load (e.g. a slow ad-hoc text match) can never clobber a
  // newer one's result after the user has already tabbed or navigated again.
  let matchRequestId = 0;

  async function loadCatalog(slug: string, token: string, requestId: number): Promise<boolean> {
    const [job, m] = await Promise.all([getJob(slug, token), getMatch(slug, token)]);
    if (requestId !== matchRequestId) return false;
    matchJob = job;
    match = m;
    return true;
  }

  async function loadMatch() {
    const requestId = ++matchRequestId;
    const token = await getToken();
    if (requestId !== matchRequestId) return;
    if (!token) {
      matchStatus = 'empty';
      matchJob = null;
      match = null;
      return;
    }
    const [tab] = await browser.tabs.query({ active: true, currentWindow: true });
    if (requestId !== matchRequestId) return;
    const url = tab?.url ?? '';

    matchStatus = 'loading';
    matchError = '';
    try {
      // Freehire's own job page → curated slug directly.
      const directSlug = freehireSlugFromUrl(url);
      if (directSlug) {
        if (!(await loadCatalog(directSlug, token, requestId))) return;
        matchStatus = 'ready';
        return;
      }

      // Any other page: recognise it as a catalog job from its URL (curated
      // card), else read the page and match against the scraped posting text.
      const catalogSlug = await findJob(url, token);
      if (requestId !== matchRequestId) return;
      const snap = catalogSlug ? null : await readSnapshot();
      if (requestId !== matchRequestId) return;
      const headline = snap?.headline || snap?.title || '';

      if (catalogSlug) {
        if (!(await loadCatalog(catalogSlug, token, requestId))) return;
      } else if (snap?.text) {
        const t = headline || 'This page';
        const m = await getMatchText(t, snap.text, token);
        if (requestId !== matchRequestId) return;
        match = m;
        matchJob = { public_slug: '', title: t, company: hostOf(url), location: '' };
      } else {
        matchStatus = 'empty';
        return;
      }
      matchStatus = 'ready';
    } catch (err) {
      if (requestId !== matchRequestId) return;
      matchError = err instanceof Error ? err.message : 'Could not load match';
      matchStatus = 'error';
    }
  }

  // The page resolved to no catalog posting: either nothing to show, or the ad-hoc text
  // match, which carries no slug. That is when freehire has something to gain from being
  // handed the page.
  let unknownPage = $derived(
    matchStatus === 'empty' || (matchStatus === 'ready' && matchJob?.public_slug === ''),
  );
  let contributing = $state(false);

  /**
   * Hands the current page to freehire. The server imports the vacancy when a link-source
   * adapter can read the page and queues the link for a maintainer when none can; either
   * way the panel says what happened, and a resolved slug re-runs the match so the curated
   * card replaces the ad-hoc one.
   */
  async function contributePage() {
    const token = await getToken();
    if (!token || contributing) return;
    contributing = true;
    try {
      const [tab] = await browser.tabs.query({ active: true, currentWindow: true });
      const url = tab?.url ?? '';
      if (!url) {
        notices.push('No page to add.');
        return;
      }
      const resolved = await resolveJob(url, token);
      notices.push(resolveNotice(resolved.status));
      if (resolved.public_slug) await loadMatch();
    } catch (err) {
      notices.push(`Could not add this page: ${err instanceof Error ? err.message : 'error'}`);
    } finally {
      contributing = false;
    }
  }

  async function readSnapshot(retries = 4): Promise<PageSnapshot | null> {
    for (let i = 0; i < retries; i++) {
      try {
        const reply = (await browser.runtime.sendMessage({
          kind: 'GET_PAGE_SNAPSHOT',
        } satisfies RuntimeMessage)) as RuntimeMessage | undefined;
        if (reply?.kind === 'PAGE_SNAPSHOT' && reply.snapshot.text) {
          return reply.snapshot;
        }
      } catch {
        // Content script not ready yet (e.g. just after an extension reload).
      }
      await new Promise((r) => setTimeout(r, 300));
    }
    return null;
  }

  function hostOf(url: string): string {
    try {
      return new URL(url).hostname.replace(/^www\./, '');
    } catch {
      return '';
    }
  }

  async function handleSignIn() {
    authBusy = true;
    authError = '';
    try {
      const token = await signIn();
      user = await fetchMe(token);
      if (!user) authError = 'Signed in, but could not load your account.';
      else {
        tools.start(token);
        void loadMatch();
      }
    } catch (err) {
      authError = err instanceof Error ? err.message : 'Sign-in failed';
    } finally {
      authBusy = false;
    }
  }

  async function handleSignOut() {
    await signOut();
    user = null;
    // Detach the wire too: it is authenticated as the user who just left.
    tools.stop();
    // Forget the conversation so a later sign-in never resumes the previous
    // user's, and clear what is on screen.
    await newChat();
  }

  /**
   * Run one turn. A turn is a single POST whose response body streams the events,
   * so there is nothing to connect, attach to, or lease — and cancelling is
   * aborting that fetch, which the backend notices on its next write.
   *
   * The user's own message is NOT painted optimistically: the backend emits
   * `user_prompt` as the turn's first frame, before the first model call, so the
   * reducer paints it just as fast and there is no echo to suppress.
   */
  async function dispatch(text: string) {
    if (sending) return;
    // Claim the turn synchronously — before the first await — so a second action
    // during `getToken()` queues out via the guard instead of double-dispatching.
    sending = true;
    chatError = '';
    const token = await getToken();
    if (!token) {
      sending = false;
      chatError = 'Sign in to chat with the agent.';
      return;
    }
    try {
      if (!sessionId) {
        sessionId = (await createSession(token)).id;
        await rememberSession(sessionId);
      }
      turn = sendTurn(sessionId, text, token, (event) => {
        chat = reduceTurnEvent(chat, event);
      });
      await turn.done;
    } catch (err) {
      // An aborted fetch is the Stop button, not a failure: the stream may drop
      // before the response headers arrive, which lands here rather than in the
      // client's own abort path.
      if (err instanceof DOMException && err.name === 'AbortError') {
        chat = reduceTurnEvent(chat, { type: 'result', stop_reason: 'cancelled' });
      } else {
        chatError = err instanceof Error ? err.message : 'Could not reach the agent.';
        // Close the open message. Without this the turn keeps its `streaming`
        // flag: the dots pulse forever and the deck skeletons never resolve, so a
        // dead connection reads as an agent still working.
        chat = reduceTurnEvent(chat, { type: 'result', stop_reason: 'error', is_error: true });
      }
    } finally {
      turn = null;
      sending = false;
    }
  }

  /** Stop a turn in flight. The client answers with a `cancelled` result, so the
   *  transcript still ends properly rather than trailing off. */
  function stopTurn() {
    turn?.cancel();
  }

  /** Start over. The old conversation stays on the server — it is in the web's
   *  session rail — so this forgets it rather than deleting it. */
  async function newChat() {
    if (sending) stopTurn();
    sessionId = null;
    await forgetSession();
    chat = initChat();
    notices = [];
    chatError = '';
  }

  function sendMessage() {
    const text = draft.trim();
    if (!text || sending) return;
    draft = '';
    void dispatch(text);
  }

  let autofilling = $state(false);

  function profileToValues(p: AutofillProfile): Record<string, string> {
    return {
      fullName: p.full_name,
      firstName: p.first_name,
      lastName: p.last_name,
      email: p.email,
      phone: p.phone,
      city: p.location,
      linkedin: p.linkedin,
      github: p.github,
      portfolio: p.portfolio,
    };
  }

  /**
   * Autofill, agent-first: freehire's agent reads the form through the wire,
   * maps the profile onto it, and fills what it can justify. The deterministic
   * filler stays as the fallback until the agent path has proven itself — it
   * only knows a fixed set of labels, but it needs nothing but this browser.
   */
  /**
   * Names the first few labels and counts the rest. A real ATS form leaves
   * dozens of fields untouched (Greenhouse alone contributes a checkbox per
   * country), and a notice that lists them all is one the user cannot read.
   */
  function nameSome(labels: string[], shown = 5): string {
    const trimmed = labels.map((l) => l.trim().replace(/\s+/g, ' ')).filter(Boolean);
    if (trimmed.length <= shown) return trimmed.join(', ');
    return `${trimmed.slice(0, shown).join(', ')} and ${trimmed.length - shown} more`;
  }

  async function autofill() {
    const token = await getToken();
    if (!token || autofilling) return;
    autofilling = true;
    try {
      const report = await runAgentAutofill(token);
      const filled = report.filled.length;
      notices.push(
        filled > 0
          ? `✓ Autofilled ${filled} field${filled === 1 ? '' : 's'} — review before submitting.`
          : 'The agent found nothing on this form it could fill from your profile.',
      );
      if (report.deferred.length > 0) {
        notices.push(`Not fillable yet (custom dropdowns): ${nameSome(report.deferred)}.`);
      }
      if (report.unmapped.length > 0) {
        notices.push(`Left for you: ${nameSome(report.unmapped)}.`);
      }
    } catch (err) {
      // The server's own sentence, not just the status: /me/autofill/run answers
      // 409 for three unrelated states, and only that sentence says which.
      notices.push(
        `Agent autofill unavailable: ${err instanceof Error ? err.message : 'error'} — using the basic filler.`,
      );
      await deterministicAutofill(token);
    } finally {
      autofilling = false;
    }
  }

  /**
   * The fallback filler, over the same frame-aware primitives the agent drives:
   * an apply form is routinely served from an ATS iframe, and a careers page
   * carrying any other iframe would otherwise be answered by whichever frame
   * replied first. Addressing questions by label rather than by position also
   * keeps the read and the write on the same question when a form re-renders
   * between them.
   */
  /**
   * The fill the user has been offered but not yet confirmed, because the page
   * does not look like it is showing an application form. Held so "Fill anyway"
   * runs the same pass rather than re-reading a page that may have moved on.
   */
  let overrideFill = $state<(() => Promise<void>) | null>(null);

  async function runOverrideFill() {
    const run = overrideFill;
    if (!run || autofilling) return;
    overrideFill = null;
    autofilling = true;
    try {
      await run();
    } finally {
      autofilling = false;
    }
  }

  async function deterministicAutofill(token: string, force = false) {
    try {
      const formReply = (await browser.runtime.sendMessage({
        kind: 'GET_FRAMED_FORM',
      } satisfies RuntimeMessage)) as RuntimeMessage | undefined;
      if (formReply?.kind !== 'FRAMED_FORM' || formReply.fields.length === 0) {
        notices.push('No form fields found on this page.');
        return;
      }

      // A careers page keeps the application behind an "Apply" button and shows a
      // job-alert signup meanwhile; filling that one silently is worse than
      // declining, so the user is told and can insist.
      if (!force && !looksLikeApplication(formReply.uploads)) {
        notices.push(
          `This doesn't look like the application form — ${formReply.fields.length} field${
            formReply.fields.length === 1 ? '' : 's'
          } are showing and none of them takes a CV. Open the application on the page, then try again.`,
        );
        overrideFill = () => deterministicAutofill(token, true);
        return;
      }
      overrideFill = null;

      // One form, not every question on the page: an application and a job-alert
      // signup each have their own "Email".
      const scoped = scopeToApplication(formReply.fields, formReply.uploads);
      const fills = planLabelFills(scoped, profileToValues(await getAutofillProfile(token)));
      if (fills.length === 0) {
        notices.push('Nothing matched your profile on this form.');
        return;
      }
      const applied = (await browser.runtime.sendMessage({
        kind: 'FILL_BY_LABEL',
        fills,
      } satisfies RuntimeMessage)) as RuntimeMessage | undefined;
      const outcomes = applied?.kind === 'FILL_OUTCOMES' ? applied.outcomes : [];
      const n = outcomes.filter((o) => o.status === 'filled').length;
      notices.push(`✓ Autofilled ${n} field${n === 1 ? '' : 's'} — review before submitting.`);

      // A custom-widget combobox commits whatever its own listbox highlights, so
      // the simple filler declines it rather than writing a wrong value.
      const deferred = outcomes.filter((o) => o.status === 'deferred_combobox').map((o) => o.label);
      if (deferred.length > 0) {
        notices.push(`Not fillable yet (custom dropdowns): ${nameSome(deferred)}.`);
      }
    } catch (err) {
      notices.push(`Autofill failed: ${err instanceof Error ? err.message : 'error'}`);
    }
  }

</script>

<div class="app">
  <header>
    <div class="top">
      <strong>freehire</strong>
      <Badge variant={sending ? 'brand' : 'outline'}>
        {sending ? 'working…' : user ? 'ready' : 'offline'}
      </Badge>
    </div>
    <div class="auth">
      {#if user}
        <span class="who">Signed in as <b>{user.email}</b></span>
        <button class="link" onclick={handleSignOut}>Sign out</button>
      {:else}
        <Button variant="primary" size="sm" onclick={handleSignIn} disabled={authBusy}>
          {authBusy ? 'Signing in…' : 'Sign in with freehire'}
        </Button>
      {/if}
    </div>
    {#if authError}
      <Alert variant="destructive">{authError}</Alert>
    {/if}
  </header>

  {#if user}
    {#if matchStatus === 'ready' && matchJob && match}
      <MatchCard job={matchJob} {match} />
    {:else if matchStatus === 'loading'}
      <div class="match-skeleton">
        <Skeleton class="h-9 w-9 rounded-lg" />
        <div class="match-skeleton-lines">
          <Skeleton class="h-3 w-2/3 rounded" />
          <Skeleton class="h-3 w-1/3 rounded" />
        </div>
      </div>
    {:else if matchStatus === 'error'}
      <EmptyState title="Match unavailable" description={matchError}>
        {#snippet action()}
          <Button variant="outline" size="sm" onclick={loadMatch}>Retry</Button>
        {/snippet}
      </EmptyState>
    {:else if matchStatus === 'empty'}
      <EmptyState title="No match yet" description="Open a job posting to see your match.">
        {#snippet action()}
          <Button variant="outline" size="sm" onclick={loadMatch}>Refresh</Button>
        {/snippet}
      </EmptyState>
    {/if}
    {#if unknownPage}
      <p class="match-hint">
        freehire doesn't have this posting.
        <button class="link" onclick={contributePage} disabled={contributing}>
          {contributing ? 'Adding…' : 'Add to freehire'}
        </button>
      </p>
    {/if}
  {/if}

  <div class="messages">
    {#each chat.messages as message, mi (mi)}
      {@const split = splitPresentingCalls(message.tools, message.streaming)}
      {#each split.decks as slot, di (di)}
        <JobDeck {slot} />
      {/each}
      {#if split.rest.length > 0}
        <ToolGroupList calls={split.rest} />
      {/if}
      {#if message.text || message.streaming}
        <div class="message {message.role}" class:errored={message.errored}>
          {message.text}{#if message.streaming && !message.text}<span class="dots">…</span>{/if}
        </div>
      {/if}
    {/each}
    {#each notices as notice, i (i)}
      <div class="message system">{notice}</div>
    {/each}
    {#if overrideFill}
      <div class="message system">
        <button class="link" onclick={runOverrideFill} disabled={autofilling}>Fill it anyway</button>
      </div>
    {/if}
    {#if chatError}
      <div class="message system err">{chatError}</div>
    {/if}
    {#if chat.messages.length === 0 && notices.length === 0}
      <p class="empty">
        {#if restoring}
          Loading your conversation…
        {:else if user}
          Ask about the page you're on — the agent can read it.
        {:else}
          Sign in to chat with the agent.
        {/if}
      </p>
    {/if}
  </div>

  <div class="composer">
    {#if user}
      <Button variant="secondary" size="sm" onclick={autofill} disabled={autofilling}>
        {autofilling ? 'Filling…' : 'Autofill'}
      </Button>
    {/if}
    {#if user && chat.messages.length > 0}
      <Button variant="secondary" size="sm" onclick={newChat} disabled={sending}>New chat</Button>
    {/if}
    <Input
      class="flex-1"
      placeholder={user ? 'Message the agent…' : 'Sign in to chat'}
      bind:value={draft}
      disabled={!user || sending}
      onkeydown={(e) => e.key === 'Enter' && sendMessage()}
    />
    {#if sending}
      <Button variant="primary" size="sm" onclick={stopTurn}>Stop</Button>
    {:else}
      <Button variant="primary" size="sm" onclick={sendMessage} disabled={!user}>Send</Button>
    {/if}
  </div>
</div>

<style>
  .app {
    display: flex;
    flex-direction: column;
    height: 100vh;
    font-size: 14px;
  }

  header {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 10px 12px;
    border-bottom: 1px solid var(--border);
  }

  .top {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .auth {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    font-size: 12px;
  }

  .who {
    color: var(--muted-foreground);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .link {
    border: none;
    background: none;
    color: var(--brand-strong);
    cursor: pointer;
    font: inherit;
    font-size: 12px;
    padding: 0;
  }

  .match-hint {
    font-size: 12px;
    color: var(--muted-foreground);
    margin: 12px;
  }

  .match-skeleton {
    display: flex;
    align-items: center;
    gap: 10px;
    margin: 12px;
  }

  .match-skeleton-lines {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .messages {
    flex: 1;
    overflow-y: auto;
    padding: 12px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .empty {
    color: var(--muted-foreground);
    text-align: center;
    margin-top: 40px;
  }

  .message {
    padding: 8px 10px;
    border-radius: 8px;
    max-width: 85%;
    white-space: pre-wrap;
    word-break: break-word;
  }

  .message.user {
    align-self: flex-end;
    background: var(--brand);
    color: var(--brand-foreground);
  }

  .message.assistant {
    align-self: flex-start;
    background: var(--muted);
    color: var(--foreground);
  }

  .message.system {
    align-self: center;
    background: var(--warning-muted);
    font-size: 12px;
    color: var(--warning-strong);
  }

  .message.system.err {
    background: color-mix(in srgb, var(--destructive) 8%, transparent);
    color: var(--destructive);
  }

  .message.assistant.errored {
    border: 1px solid color-mix(in srgb, var(--destructive) 40%, transparent);
  }

  .dots {
    opacity: 0.5;
  }

  .composer {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 10px;
    border-top: 1px solid var(--border);
  }
</style>
