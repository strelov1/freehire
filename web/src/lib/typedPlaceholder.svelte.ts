import { untrack } from 'svelte';
import {
  completeWord,
  typedTail,
  typewriterStart,
  typewriterStep,
  type TypewriterState,
} from '#lib/placeholderRoles.js';

/** HeaderSearch's placeholder: a plain string, or `{ prefix, roles }` typed one role at a time. */
export type SearchPlaceholder = string | { prefix: string; roles: string[] };

// The typed placeholder behind HeaderSearch's empty box. Called during component
// initialisation (it registers the effects that drive it) with a getter for the prop, and
// read through `shown`; `stop` and `resume` are the box's focus and autofocus handlers.
export function typedPlaceholder(placeholder: () => SearchPlaceholder) {
  // ---- the typed placeholder ----
  //
  // Runs only while the box is empty and has never been touched. The first focus or
  // keystroke stops it for the rest of the visit — text moving under the cursor while a
  // query is being composed is the failure an animated placeholder invites.
  //
  // Only the tail moves; `prefix` is never rewritten, so the sentence stays readable and
  // the eye has one place to look. The words and the stepping both live in
  // lib/placeholderRoles.ts, which is where they can be tested without a browser.
  const typing = $derived.by(() => {
    const p = placeholder();
    return typeof p === 'string' ? null : p;
  });

  // Capturing just the initial value is the intent: this is where the animation STARTS,
  // and the server renders it. A caller that later swaps its word list is handled by the
  // stepper instead — a role index the new list has no word for steps past on the next
  // tick rather than needing this seed to be recomputed.
  const initial = placeholder();
  let typewriter = $state<TypewriterState>(typewriterStart(typeof initial === 'string' ? [] : initial.roles));
  let typingStopped = $state(false);

  function stopTyping() {
    if (typingStopped || !typing) return;
    // Finish the word on the way out. Freezing on "Devo" reads as a typo rather than as
    // an animation that stopped, and it would do so at the exact moment the visitor's
    // attention arrived on the field.
    typewriter = completeWord(typewriter, typing.roles);
    typingStopped = true;
  }

  /** Undo a stop that no visitor asked for — see HeaderSearch's autofocus effect. */
  function resumeTyping() {
    if (!typing) return;
    // Both halves, not just the flag: stopTyping also FILLED IN the current word, so
    // clearing the flag alone would leave the animation resuming from a finished word
    // and skip typing the very first one — on the homepage hero, the one surface this
    // exists for.
    typewriter = typewriterStart(typing.roles);
    typingStopped = false;
  }

  // Sampled AND subscribed: the OS-level setting can be turned on mid-visit, and reading
  // it once would leave that visitor with a timer already running for the rest of it.
  let reduceMotion = $state(false);
  $effect(() => {
    const query = window.matchMedia('(prefers-reduced-motion: reduce)');
    reduceMotion = query.matches;
    const onChange = (e: MediaQueryListEvent) => (reduceMotion = e.matches);
    query.addEventListener('change', onChange);
    return () => query.removeEventListener('change', onChange);
  });

  $effect(() => {
    if (typingStopped || !typing) return;
    const roles = typing.roles;
    // Under reduced motion nothing is scheduled at all and the first role simply stands.
    if (reduceMotion || roles.length === 0) return;

    // One chained timeout rather than an interval: every step has its own delay (a typed
    // character, a faster deleted one, and the long hold on a finished word), so a fixed
    // tick would have to be the shortest of them and then count its way to the others.
    //
    // `untrack` is what makes that chain real. Read plainly, the seed below would
    // subscribe this effect to the very state its own callback writes: each character
    // would invalidate the effect, the cleanup would cancel the link already scheduled,
    // and the animation would be driven by re-runs at the mercy of flush latency rather
    // than by the delays computed here.
    let timer: ReturnType<typeof setTimeout> | undefined;
    const schedule = (from: TypewriterState) => {
      const { next, delayMs } = typewriterStep(from, roles);
      timer = setTimeout(() => {
        // Untracked on the WRITE as well as on the seed below, and for the same reason
        // stated the other way round: this chain drives itself, so neither end of it is
        // part of any reaction. Svelte's `set` refuses a write whose active reaction is a
        // derived or a block effect (svelte/src/internal/client/reactivity/sources.js),
        // and `untrack` is what says this assignment answers to neither —
        // state_unsafe_mutation reached production from this line (FREEHIRE-WEB-26) and
        // took the header down with it, which a placeholder animation must never be able
        // to do whatever raced it.
        untrack(() => {
          typewriter = next;
        });
        schedule(next);
      }, delayMs);
    };
    schedule(untrack(() => typewriter));
    return () => clearTimeout(timer);
  });

  // Server-rendered markup carries the prefix with an empty tail, since the effect above
  // is client-only — so the sentence is already there and only its last word arrives.
  // Under reduced motion the first role is shown whole instead.
  const shown = $derived.by(() => {
    if (!typing) return placeholder() as string;
    if (reduceMotion && !typingStopped) return typing.prefix + (typing.roles[0] ?? '');
    return typing.prefix + typedTail(typewriter, typing.roles);
  });

  return {
    get shown() {
      return shown;
    },
    stop: stopTyping,
    resume: resumeTyping,
  };
}
