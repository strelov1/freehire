import { untrack } from 'svelte';
import { OperatorLookup } from '#lib/operatorLookup.js';
import { activeOperator, type OperatorOption } from '#lib/searchOperators.js';

type Caret = { text: string; caret: number };

const OPERATOR_LIMIT = 8;

/**
 * The header search's field operators: `title:`, `company:` and `source:` are a typing
 * shortcut for filters the feed already has (see #lib/searchOperators). Inside an
 * operator's value the panel offers only that kind; on Enter the operators become those
 * filters. Construct during component initialisation: it runs an effect.
 */
export class OperatorPanel {
  // The last answer, with the token it answers — so an EMPTY answer can be told apart
  // from one that has not arrived yet.
  // `options` is null when nothing was asked (a title too short to complete) or the
  // lookup failed — neither is "nothing matches", and saying so would be a lie.
  #answer = $state.raw<{
    key: string;
    value: string;
    options: OperatorOption[] | null;
  } | null>(null);

  /** The operator lookups and the company names they have resolved: see operatorLookup.ts. */
  readonly lookup: OperatorLookup;

  // The operator the caret is in, read live so the panel switches mode on the keystroke
  // rather than after the debounce. The options themselves follow the settled draft.
  readonly live = $derived.by(() => (this.#enabled() ? activeOperator(this.#draft().text, this.#draft().caret) : null));

  // Inside a value that names nothing, say so. An empty panel would vanish, and after an
  // Enter that stopped on this token that reads as the box ignoring the key.
  readonly miss = $derived.by(
    () =>
      this.live !== null &&
      this.live.value !== '' &&
      this.#answer?.key === this.live.key &&
      this.#answer.value === this.live.value &&
      this.#answer.options?.length === 0,
  );

  // The rows for the operator the caret is in — null outside one. Only the answer for
  // the KIND the caret is in: until the next pause settles, the last answer may be about
  // a different operator.
  readonly options = $derived.by(() => {
    if (!this.live) return null;
    return this.#answer?.key === this.live.key ? (this.#answer.options ?? []) : [];
  });

  readonly #draft: () => Caret;
  readonly #enabled: () => boolean;

  constructor(
    draft: () => Caret,
    settled: () => Caret,
    enabled: () => boolean,
    limits: { companiesFetch: number; minChars: number },
  ) {
    this.#draft = draft;
    this.#enabled = enabled;
    this.lookup = new OperatorLookup({
      companiesFetch: limits.companiesFetch,
      optionLimit: OPERATOR_LIMIT,
      minChars: limits.minChars,
    });

    // The settled token as a string, so moving the caret WITHIN an unchanged value — an
    // arrow key inside `company:goo` — does not abort and resend the same lookup.
    const settledOperator = $derived.by(() => {
      const s = settled();
      const op = enabled() ? activeOperator(s.text, s.caret) : null;
      return op ? `${op.key}:${op.start}:${op.value}` : '';
    });

    $effect(() => {
      if (!settledOperator) {
        this.#answer = null;
        return;
      }
      const op = untrack(() => activeOperator(settled().text, settled().caret));
      if (!op) {
        this.#answer = null;
        return;
      }
      // A newer pause aborts this lookup, the same abandonment rule as the box's fetches.
      const ac = new AbortController();
      void this.lookup
        .optionsFor(op, ac.signal)
        .catch(() => null)
        .then((opts) => {
          if (!ac.signal.aborted) this.#answer = { key: op.key, value: op.value, options: opts };
        });
      return () => ac.abort();
    });
  }
}
