<script lang="ts">
  import { onMount } from 'svelte';
  import { browser } from 'wxt/browser';
  import { type RuntimeMessage, type LabelFill } from '../../lib/protocol';
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
    getAutofillProfile,
    runAgentAutofill,
    resolveJob,
    resolveNotice,
    type FreehireJob,
    type JobMatch,
    type AutofillProfile,
  } from '../../lib/freehire';
  import {
    planLabelFills,
    looksLikeApplication,
    scopeToApplication,
    formatAuthorizedCountries,
  } from '../../lib/form';
  import { buildPlan, markAnswered, showsApplicationForm, type ApplyPlan, type PlanItem } from '../../lib/applyPlan';
  import { startWalk, nextStep, applyStep, skipStep, stopWalk, type Walk } from '../../lib/walk';
  import { ToolChannel } from '../../lib/tools/client';
  import { activeTabPage } from '../../lib/tools/page';
  import MatchCard from './MatchCard.svelte';
  import ApplyPlanCard from './ApplyPlan.svelte';
  import ToolGroupList from './ToolGroupList.svelte';
  import JobDeck from './JobDeck.svelte';
  import { splitPresentingCalls } from '../../lib/assistant/deck';
  import { Alert, Badge, Button, Card, EmptyState, Input, Skeleton, TabStrip, tabStripId } from 'freehire-design-system';
  import { ArrowUp, Plus, Square, RectangleEllipsis } from '@lucide/svelte';

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

  // The page the CURRENT conversation is about (see `pageKey`). A job posting's
  // chat has no reason to carry over to a different page — there is nothing left
  // to continue — so this is compared against the active tab on every tab switch
  // and reload, and a mismatch resets the conversation instead of resuming it.
  // Null until the first page is known.
  let chatPageKey: string | null = null;

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

  // "Match" carries the current page's job info and its page-scoped actions
  // (Autofill, Add to freehire); "Chat" carries the conversation. Split into tabs
  // because the chat transcript needs the whole panel height to itself to scroll
  // — sharing it with the match card left too little room to reach the composer.
  const PANEL_ID = 'sidepanel-panel';
  let activeTab = $state<'match' | 'chat'>('match');

  onMount(() => {
    // The conversation is created lazily on the first message, so an idle panel
    // starts nothing; a conversation held earlier is repainted here.
    void restoreSession();

    // Re-run the match when the user switches tabs or a page finishes loading,
    // so the card tracks whatever job page is in front — like the reference.
    // The same event is what notices a genuine page change for the chat: see
    // `handlePageChange`.
    const refresh = () => {
      if (user) void handlePageChange();
    };
    browser.tabs.onActivated.addListener(refresh);
    const onUpdated = (_id: number, info: { status?: string }) => {
      if (info.status === 'complete') refresh();
    };
    browser.tabs.onUpdated.addListener(onUpdated);

    // An answer the user types on the page themselves has to move the panel's
    // counter, or the plan reports the form as less finished than it is. The page
    // says only that something changed (debounced there); the plan is rebuilt from
    // a fresh read, which is the one account that cannot drift.
    const onPageMessage = (message: RuntimeMessage) => {
      if (message.kind === 'FORM_CHANGED') void refreshPlan();
    };
    browser.runtime.onMessage.addListener(onPageMessage);

    // The panel regaining focus is the other moment worth re-reading on: the user
    // has just been on the page, quite possibly answering a question there.
    const onFocus = () => {
      if (user) void refreshPlan();
    };
    window.addEventListener('focus', onFocus);

    return () => {
      turn?.cancel();
      tools.stop();
      browser.tabs.onActivated.removeListener(refresh);
      browser.tabs.onUpdated.removeListener(onUpdated);
      browser.runtime.onMessage.removeListener(onPageMessage);
      window.removeEventListener('focus', onFocus);
    };
  });

  /** Identifies the page a conversation is about, for `chatPageKey`. The query
   *  string is dropped so a tracking-parameter change on the same posting is not
   *  read as a different one; the path is kept because that is what actually
   *  distinguishes one job posting's URL from another's, on freehire's own job
   *  pages and on an ATS's alike. */
  function pageKey(url: string): string {
    try {
      const u = new URL(url);
      return `${u.origin}${u.pathname}`;
    } catch {
      return url;
    }
  }

  async function currentPageKey(): Promise<string> {
    const [tab] = await browser.tabs.query({ active: true, currentWindow: true });
    return pageKey(tab?.url ?? '');
  }

  /** Strips the fragment and any embedded credentials from a URL before it goes to
   *  the server for a catalog lookup — neither affects which job a page is, and a
   *  fragment can carry an SSO token on some ATS redirects. Query parameters are
   *  kept: Greenhouse's embedded application form encodes the board and job id
   *  there (`for`, `token`), which `sources.RefFromURL` depends on. */
  function sanitizeForLookup(url: string): string {
    try {
      const u = new URL(url);
      return `${u.origin}${u.pathname}${u.search}`;
    } catch {
      return url;
    }
  }

  /**
   * Runs on every tab switch and page load. A genuine change of page clears
   * whatever conversation is on screen — there is nothing on the new page for it
   * to continue — before the match card is refreshed for the new page. The very
   * first call (chatPageKey still null) only establishes the baseline; there is
   * nothing yet to reset it against.
   *
   * The clear is local only (`resetChat`, not `newChat`): the page just left
   * might still have a perfectly good remembered conversation, and switching
   * away from it is not the user asking to discard it — only the Reset button
   * and sign-out are. Immediately after, the new page's OWN remembered
   * conversation (if any) is offered the normal restore path, so tabbing A → B →
   * A resumes A's conversation rather than finding it erased.
   */
  async function handlePageChange() {
    const key = await currentPageKey();
    formFilled = false;
    if (chatPageKey !== null && key !== chatPageKey) {
      resetChat();
      const token = await getToken();
      if (token) void restoreConversation(token, key);
    }
    chatPageKey = key;
    void loadMatch();
    void refreshPlan();
  }

  async function restoreSession() {
    const token = await getToken();
    if (!token) return;
    user = await fetchMe(token);
    if (!user) return;
    tools.start(token);
    const key = await currentPageKey();
    chatPageKey = key;
    void loadMatch();
    void refreshPlan();
    void restoreConversation(token, key);
  }

  /**
   * Repaint the conversation this panel was holding. The transcript is replayed
   * through the same reducer a live turn folds through, so history and a running
   * turn cannot render differently.
   *
   * A remembered conversation about a different page is not repainted here — it
   * is simply not what this page's chat should show — but it is left in storage
   * rather than forgotten: it is exactly right for whatever page it WAS about,
   * and staying there is what lets tabbing back to that page resume it.
   *
   * A conversation the server no longer has (deleted from the web) is not an error
   * the user can act on from here — forget it and let the next message start a
   * fresh one.
   */
  async function restoreConversation(token: string, key: string) {
    const remembered = await recallSession();
    if (!remembered) return;
    if (remembered.pageKey !== key) return;
    restoring = true;
    try {
      const { messages } = await getSession(remembered.id, token);
      // The composer unlocks as soon as the user is known, so a message can be
      // sent while this read is in flight — and that message created its own
      // conversation. Adopting the remembered one now would point the panel at A
      // while storage holds B, and lose the exchange the user just watched.
      if (sessionId) return;
      sessionId = remembered.id;
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

      // Any other page: recognise it as a catalog job from its URL (curated card),
      // else there is nothing to show. No ad-hoc scrape-and-match call here — that
      // used to run automatically on every tab the panel happened to be open on,
      // job posting or not, which is both wasted work and a wasted LLM call for
      // the vast majority of pages. `contributePage` (the "Add vacancy" action on
      // the empty state) is the explicit, on-demand equivalent.
      const catalogSlug = await findJob(sanitizeForLookup(url), token);
      if (requestId !== matchRequestId) return;
      if (catalogSlug) {
        if (!(await loadCatalog(catalogSlug, token, requestId))) return;
        matchStatus = 'ready';
        return;
      }
      matchStatus = 'empty';
    } catch (err) {
      if (requestId !== matchRequestId) return;
      matchError = err instanceof Error ? err.message : 'Could not load match';
      matchStatus = 'error';
    }
  }

  let contributing = $state(false);

  /**
   * Hands the current page to freehire. The server imports the vacancy when a link-source
   * adapter can read the page and queues the link for a maintainer when none can; either
   * way the panel says what happened, and a resolved slug re-runs the match so the curated
   * card replaces the ad-hoc one.
   */
  async function contributePage() {
    // Claimed before the first await: `disabled` only follows the flag, so a second
    // activation landing while the token is being read would otherwise pass the guard
    // too and post the page twice — two imports, and two charges against the rate limit
    // the endpoint carries.
    if (contributing) return;
    contributing = true;
    try {
      const token = await getToken();
      if (!token) return;
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

  async function handleSignIn() {
    authBusy = true;
    authError = '';
    try {
      const token = await signIn();
      user = await fetchMe(token);
      if (!user) authError = 'Signed in, but could not load your account.';
      else {
        tools.start(token);
        chatPageKey = await currentPageKey();
        void loadMatch();
        void refreshPlan();
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
    // No page is "current" while signed out — re-established by restoreSession
    // or handleSignIn on the next sign-in, same as on first mount.
    chatPageKey = null;
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
        const key = chatPageKey ?? (await currentPageKey());
        sessionId = (await createSession(token)).id;
        await rememberSession(sessionId, key);
        chatPageKey = key;
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

  /** Clears what the panel is showing, without touching the remembered session in
   *  storage — used where the underlying conversation may still be worth keeping
   *  (a page change; see `handlePageChange`). `newChat` below is the same clear
   *  plus the deliberate forget. */
  function resetChat() {
    if (sending) stopTurn();
    sessionId = null;
    chat = initChat();
    notices = [];
    chatError = '';
  }

  /** Start over — the Reset button. The old conversation stays on the server — it
   *  is in the web's session rail — so this forgets the panel's local pointer to
   *  it rather than deleting it. chatPageKey is left as-is: the button does not
   *  navigate anywhere, so the current page is still the current page. */
  async function newChat() {
    resetChat();
    await forgetSession();
  }

  function sendMessage() {
    const text = draft.trim();
    if (!text || sending) return;
    draft = '';
    void dispatch(text);
  }

  let autofilling = $state(false);

  // ── The application-form plan ──────────────────────────────────────────────
  //
  // The panel's standing account of the form in front of the user: what it asks,
  // what is answered, how far the required questions are along. It exists before
  // any fill and outlives every one, which is the difference from the notices —
  // those describe one action and are gone by the next.

  let plan = $state<ApplyPlan | null>(null);
  /** The question a walk is filling right now, by label. */
  let fillingLabel = $state<string | null>(null);
  // Reads of the form are serialised rather than raced. A request id (the
  // `matchRequestId` pattern) was the first attempt and it deadlocked on a real
  // ATS page: the page announces changes continuously, so every read found its id
  // already superseded by the next one and returned before assigning anything —
  // the checklist never appeared at all. One read at a time, with a re-read queued
  // if anything arrived meanwhile, cannot starve and cannot land stale.
  let planReading = false;
  let planStale = false;
  /** How many labelled questions the last read found, whether or not they added
   *  up to an application. It is what the panel says when it shows no checklist,
   *  so "nothing appeared" can be told from "nothing was found". */
  let questionsSeen = $state(0);
  /** Why the last read produced nothing, when it failed outright. */
  let planError = $state('');
  /** True once a walk has written into this page's form. It outranks every guess
   *  about whether the page is showing an application: it accepted values. Reset
   *  by a page change, with the plan. */
  let formFilled = $state(false);

  /** Reads the page's form and rebuilds the plan, or clears it for a page that
   *  is not showing an application. */
  async function refreshPlan() {
    if (!user) return;
    if (planReading) {
      planStale = true;
      return;
    }
    planReading = true;
    try {
      do {
        planStale = false;
        const reply = (await browser.runtime.sendMessage({
          kind: 'GET_FRAMED_FORM',
        } satisfies RuntimeMessage)) as RuntimeMessage | undefined;
        applyRead(reply);
      } while (planStale);
    } catch (err) {
      // A read that could not happen is not a page without a form: say so rather
      // than clearing a checklist the user is looking at.
      planError = err instanceof Error ? err.message : 'could not read the form';
    } finally {
      planReading = false;
    }
  }

  /** Turns one form read into the plan, or into the reason there is none. */
  function applyRead(reply: RuntimeMessage | undefined) {
    planError = '';
    if (reply?.kind !== 'FRAMED_FORM') {
      questionsSeen = 0;
      plan = null;
      return;
    }
    questionsSeen = reply.fields.filter((f) => f.label.trim() !== '').length;
    if (!showsApplicationForm(reply.fields, reply.uploads, { filled: formFilled })) {
      plan = null;
      return;
    }
    // Scoped to the one form the upload identifies, where there is one: an
    // application and a job-alert signup each have their own "Email". With no
    // upload to scope by — every step of an ATS form after the first — the page's
    // questions ARE the form.
    plan = buildPlan(
      reply.uploads.length > 0 ? scopeToApplication(reply.fields, reply.uploads) : reply.fields,
    );
  }

  /** Sends the user to one question: the page scrolls there and takes the cursor. */
  async function revealItem(item: PlanItem) {
    const reply = (await browser.runtime.sendMessage({
      kind: 'REVEAL_FIELD',
      request: { label: item.label, frame: item.frame, form: item.form, focus: true },
    } satisfies RuntimeMessage)) as RuntimeMessage | undefined;
    if (reply?.kind === 'REVEAL_RESULT' && !reply.found) {
      notices.push(`"${item.label}" is no longer on this page — the form may have moved on.`);
    }
  }

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
      authorizedCountries: formatAuthorizedCountries(p.authorized_countries),
      visaSponsorshipNeeded: p.visa_sponsorship_needed,
      desiredSalary: p.desired_salary,
      noticePeriod: p.notice_period,
      willingToRelocate: p.willing_to_relocate,
      age18OrOlder: p.age_18_or_older,
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

  /** How long each filled question stays in view before the walk moves on. It is
   *  there for the eye: without it the walk is a flicker, which is the batch fill
   *  this replaced. */
  const WALK_STEP_MS = 300;

  /** Set while a walk runs, so Stop can end it between steps. */
  let walkStop = $state(false);

  function pause(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }

  /** Ticks one question off the plan, narrowed by whatever address the caller
   *  has: a fill knows its frame and form, the agent's report knows the label
   *  alone. An address that matches nothing leaves the plan as it was. */
  function tickOff(label: string, frame?: number, form?: number) {
    if (!plan) return;
    const item = plan.items.find(
      (i) =>
        i.label === label &&
        (frame === undefined || i.frame === frame) &&
        (form === undefined || i.form === form),
    );
    if (item) plan = markAnswered(plan, item.key);
  }

  /**
   * Works through `fills` one question at a time, each one revealed as its value
   * lands, ticking the plan off as it goes. Returns what it managed — the caller
   * owns the closing sentence, because the two callers reach the walk from
   * different places (its own plan, or the agent's report).
   */
  async function walkFills(fills: LabelFill[]): Promise<Walk<LabelFill>> {
    let walk = startWalk(fills);
    try {
      // `nextStep` is the continuation test: it ends the walk both when the list
      // runs out and when Stop has been pressed.
      for (let fill = nextStep(walk); fill !== null; fill = nextStep(walk)) {
        fillingLabel = fill.label;
        const applied = (await browser.runtime.sendMessage({
          kind: 'FILL_BY_LABEL',
          fills: [fill],
          reveal: true,
        } satisfies RuntimeMessage)) as RuntimeMessage | undefined;
        const outcome = applied?.kind === 'FILL_OUTCOMES' ? applied.outcomes[0] : undefined;
        if (outcome?.status === 'filled') {
          walk = applyStep(walk, fill);
          formFilled = true;
          // Ticked off from what was just written, not from a fresh read: the
          // page's own change notice is debounced, so re-reading here would leave
          // the counter a step behind the value on screen.
          tickOff(fill.label, fill.frame, fill.form);
        } else {
          walk = skipStep(walk, fill);
        }
        if (walkStop) return stopWalk(walk);
        await pause(WALK_STEP_MS);
      }
      return walk;
    } finally {
      fillingLabel = null;
    }
  }

  /** Plays a list of labels the agent says it filled back over the page, so the
   *  user sees what changed. Nothing is written — the agent already did that. */
  async function walkReported(labels: string[]) {
    try {
      for (const label of labels) {
        if (walkStop) return;
        fillingLabel = label;
        await browser.runtime.sendMessage({
          kind: 'REVEAL_FIELD',
          request: { label },
        } satisfies RuntimeMessage);
        tickOff(label);
        await pause(WALK_STEP_MS);
      }
    } finally {
      // Whatever ended the walk — the list, Stop, or a throw — no question is
      // being filled once it is over, and a stuck spinner would say otherwise.
      fillingLabel = null;
    }
  }

  async function autofill() {
    const token = await getToken();
    if (!token || autofilling) return;
    autofilling = true;
    walkStop = false;
    try {
      // Read the form first: the walk ticks questions off the plan, and a page
      // whose form arrived after the last read (an ATS step change, which fires
      // no page load) would otherwise be walked with nothing to tick.
      await refreshPlan();
      const report = await runAgentAutofill(token);
      // The agent filled the page itself, server-side. Play its report back so
      // the user watches what changed rather than finding it later.
      if (report.filled.length > 0) formFilled = true;
      await walkReported(report.filled);
      const filled = report.filled.length;
      notices.push(
        filled > 0
          ? `✓ Autofilled ${filled} field${filled === 1 ? '' : 's'} — review before submitting.`
          : 'The agent found nothing on this form it could fill from your profile.',
      );
      if (report.deferred.length > 0) {
        notices.push(`Not fillable yet (custom dropdowns): ${nameSome(report.deferred)}.`);
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
      const walk = await walkFills(fills);
      // The form has now accepted values, which is stronger evidence than
      // anything its markup says — so read it again and let the checklist appear.
      await refreshPlan();
      const n = walk.done.length;
      // What was left unanswered is not listed here: the checklist above shows
      // exactly which questions those are, and naming thirty of them in a notice
      // is the wall of text this feature replaced.
      notices.push(
        walk.stopped
          ? `Stopped after ${n} field${n === 1 ? '' : 's'} — what was filled stays.`
          : `✓ Autofilled ${n} field${n === 1 ? '' : 's'} — review before submitting.`,
      );
    } catch (err) {
      notices.push(`Autofill failed: ${err instanceof Error ? err.message : 'error'}`);
    }
  }

</script>

<div class="app">
  <header>
    <div class="top">
      <span class="brand">
        <img src="/icon/32.png" alt="" class="brand-mark" width="18" height="18" />
        <strong>freehire</strong>
        <!-- The build, in the one place both of us can see it: a side panel keeps
             its script until it is closed, so "did the reload take?" is otherwise
             unanswerable from a screenshot. -->
        <span class="build">v{browser.runtime.getManifest().version}</span>
      </span>
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

  <TabStrip
    class="tab-strip"
    tabs={[
      { id: 'match', label: 'Match' },
      { id: 'chat', label: 'Chat' },
    ]}
    active={activeTab}
    onSelect={(id) => {
      activeTab = id;
      // Coming back to Match is a moment the user expects the panel to be current:
      // the form may have moved on (a step, an expanded Apply) while they were in
      // the chat, and nothing else would have told us.
      if (id === 'match') void refreshPlan();
    }}
    label="Panel sections"
    panelId={PANEL_ID}
  />

  <div class="tab-panel" role="tabpanel" id={PANEL_ID} aria-labelledby={tabStripId(PANEL_ID, activeTab)}>
    {#if activeTab === 'match'}
      <div class="match-panel">
        <div class="match-scroll">
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
              <!-- Not an empty state but an offer: a page freehire does not carry yet is
                   the one moment the panel can ask for something useful, so it leads with
                   the action and says what it buys — rather than reporting a miss. -->
              <Card class="add-job">
                <Button
                  class="add-job-cta"
                  variant="primary"
                  size="lg"
                  onclick={contributePage}
                  disabled={contributing}
                >
                  <Plus class="size-4" />
                  {contributing ? 'Adding this job…' : 'Add this job in one click'}
                </Button>
                <p class="add-job-hint">See your match score and tailor your CV</p>
              </Card>
            {/if}

            <!-- The account of the form the user is standing in front of. It is
                 independent of the match: a page can show an application freehire
                 does not carry a posting for, and the checklist is just as useful
                 there. -->
            {#if !plan && planError}
              <p class="no-plan">Could not read this page's form: {planError}</p>
            {:else if !plan && questionsSeen > 0}
              <!-- The page asks questions but they did not add up to an
                   application. Saying so beats an empty space the user has to
                   guess about. -->
              <p class="no-plan">
                {questionsSeen} field{questionsSeen === 1 ? '' : 's'} on this page — not enough to
                read as an application form.
              </p>
            {/if}
            {#if plan}
              <ApplyPlanCard
                {plan}
                filling={fillingLabel}
                walking={autofilling}
                onReveal={revealItem}
                onCancel={() => (walkStop = true)}
              />
            {/if}

            {#each notices as notice, i (i)}
              <div class="message system">{notice}</div>
            {/each}
            {#if overrideFill}
              <div class="message system">
                <button class="link" onclick={runOverrideFill} disabled={autofilling}>Fill it anyway</button>
              </div>
            {/if}
          {:else}
            <p class="empty">Sign in to see your match for this page.</p>
          {/if}
        </div>

        {#if user && matchStatus === 'ready' && matchJob && matchJob.public_slug !== ''}
          <div class="match-footer">
            {#if autofilling}
              <!-- The same button, not a second one beside it: while a walk runs
                   there is exactly one thing to do with it, and a disabled
                   "Filling…" left the user watching something they could not
                   call off. -->
              <Button class="w-full" variant="outline" size="lg" onclick={() => (walkStop = true)} disabled={walkStop}>
                {walkStop ? 'Stopping…' : 'Stop'}
                <Square class="size-4" />
              </Button>
            {:else}
              <Button class="w-full" variant="primary" size="lg" onclick={autofill}>
                Autofill
                <RectangleEllipsis class="size-4" />
              </Button>
            {/if}
          </div>
        {/if}
      </div>
    {:else}
      <div class="chat-panel">
        {#if user}
          <div class="chat-toolbar">
            <Button variant="ghost" size="sm" onclick={newChat} disabled={sending}>Reset</Button>
          </div>
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
          {#if chatError}
            <div class="message system err">{chatError}</div>
          {/if}
          {#if chat.messages.length === 0}
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
          <Input
            class="flex-1"
            placeholder={user ? 'Message the agent…' : 'Sign in to chat'}
            bind:value={draft}
            disabled={!user || sending}
            onkeydown={(e) => e.key === 'Enter' && sendMessage()}
          />
          {#if sending}
            <Button
              class="rounded-full"
              variant="primary"
              size="icon"
              aria-label="Stop the assistant"
              onclick={stopTurn}
            >
              <Square class="size-3.5" fill="currentColor" />
            </Button>
          {:else}
            <Button
              class="rounded-full"
              variant="primary"
              size="icon"
              aria-label="Send message"
              onclick={sendMessage}
              disabled={!user}
            >
              <ArrowUp class="size-4" strokeWidth={2.5} />
            </Button>
          {/if}
        </div>
      </div>
    {/if}
  </div>
</div>

<style>
  .app {
    display: flex;
    flex-direction: column;
    height: 100vh;
    overflow: hidden;
    font-size: 14px;
  }

  header {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 10px 12px;
  }

  .top {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .brand-mark {
    border-radius: 4px;
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

  /* :global — `class="tab-strip"` is forwarded onto TabStrip's own root element,
   * which carries that component's scoping hash, not App.svelte's; a scoped
   * selector here would silently match nothing, which is exactly what dropped
   * the strip's inset. */
  :global(.tab-strip) {
    padding: 0 12px;
    flex-shrink: 0;
  }

  /* The panel that actually has to fill the space between the tab strip and the
   * viewport's bottom edge — min-height: 0 overrides a flex item's default of
   * shrinking no further than its content, which is what silently broke internal
   * scrolling here: without it this item grew to fit the transcript instead of
   * scrolling it, pushing the composer off screen. */
  /* min-height AND min-width: 0 override a flex item's default of never
   * shrinking below its content's intrinsic size, in either axis. The height
   * half is what made the chat transcript scroll instead of pushing the
   * composer off-screen (see .messages below); the width half is the same bug
   * sideways — some un-wrapped child (a badge row, a long value) can otherwise
   * inflate this column wider than the viewport, and everything sized against
   * it with `width: 100%` — including the Autofill button — inflates to match
   * and then gets silently clipped at .app's own edge, which reads as "no
   * padding" even though the padding below is real. */
  .tab-panel {
    flex: 1;
    min-height: 0;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  /* Plain flex wrapper, same shape as .chat-panel: .match-scroll is the part
   * that scrolls, .match-footer (the Autofill button) stays pinned below it,
   * same split as .messages/.composer on the Chat tab. */
  .match-panel {
    flex: 1;
    min-height: 0;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  .match-scroll {
    flex: 1;
    min-height: 0;
    min-width: 0;
    overflow-y: auto;
    padding: 12px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  /* :global — a direct child can be a component's own root (MatchCard's card),
   * which carries that component's own scope hash, not this one; see the
   * .tab-strip note above for the same reason. Without this, a flex column with
   * `overflow-y: auto` still shrinks its children to fit before it scrolls —
   * content taller than the visible area (a fully loaded MatchCard, well over a
   * short skeleton) proportionally squashed every child's height. `flex-shrink:
   * 0` makes .match-scroll actually scroll instead of squeezing its children. */
  .match-scroll > :global(*) {
    flex-shrink: 0;
  }

  .match-footer {
    flex-shrink: 0;
    padding: 10px 12px;
    border-top: 1px solid var(--border);
  }

  .chat-panel {
    flex: 1;
    min-height: 0;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  /* :global — Card renders its own root, which carries the design system's scope
   * hash rather than this component's; the same reason .tab-strip and
   * .match-scroll > * are written this way. */
  :global(.add-job) {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
    padding: 20px 16px;
    text-align: center;
  }

  /* A pill, not the square primary button: this is an invitation on an otherwise
   * empty panel, and the panel's own committed action (Autofill, pinned below) is
   * the squared one — keeping the shapes apart keeps the hierarchy readable. */
  :global(.add-job-cta) {
    border-radius: 9999px;
    white-space: normal;
    max-width: 100%;
  }

  .add-job-hint {
    color: var(--muted-foreground);
    font-size: 13px;
    line-height: 1.4;
  }

  .match-skeleton {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .match-skeleton-lines {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .messages {
    flex: 1;
    min-height: 0;
    min-width: 0;
    overflow-y: auto;
    padding: 12px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  /* Same fix as .match-panel above, same reason: without it a long transcript
   * would squash individual messages instead of just scrolling past them. */
  .messages > :global(*) {
    flex-shrink: 0;
  }

  .build {
    font-size: 11px;
    color: var(--muted-foreground);
  }

  .no-plan {
    font-size: 12px;
    color: var(--muted-foreground);
    padding: 0 2px;
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

  .chat-toolbar {
    display: flex;
    justify-content: flex-end;
    padding: 6px 10px 0;
    flex-shrink: 0;
  }

  .composer {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 10px;
    border-top: 1px solid var(--border);
  }
</style>
