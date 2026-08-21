import { describe, expect, it } from 'vitest';
import { propsFrom } from './validate-docs.mjs';

// The names the docs have to use. A prop is what the *call site* writes, which is
// not always what the component destructures into — `class: className` is written
// `class`, and the rest spread is written by naming it.
describe('propsFrom', () => {
  it('reads a multi-line destructure with defaults', () => {
    expect(
      propsFrom(`let {
        variant = 'secondary',
        size = 'md',
        children,
      }: Props = $props();`),
    ).toEqual(['variant', 'size', 'children']);
  });

  it('reads a single-line destructure', () => {
    expect(propsFrom(`let { code, label }: { code: string; label: string } = $props();`)).toEqual([
      'code',
      'label',
    ]);
  });

  // The call site writes class=, never className=, so className is the wrong name
  // to hold the docs to.
  it('names a renamed prop by the name callers write', () => {
    expect(propsFrom(`let { class: className = 'size-4' } = $props();`)).toEqual(['class']);
  });

  // Input documents its passthrough as a prop called '...rest'. Keep that spelling:
  // it is the one entry that tells a reader the component forwards anything else.
  it('names the rest spread the way the docs already do', () => {
    expect(propsFrom(`let { value = $bindable(), ...rest } = $props();`)).toEqual([
      'value',
      '...rest',
    ]);
  });

  // The inline type annotation is a second brace block between the destructure and
  // = $props(). Its members are types, not props, and counting them would demand
  // docs for entries no caller can pass.
  it('stops at the destructure, not at the inline type that follows it', () => {
    expect(
      propsFrom(`let {
        open = $bindable(false),
        onConfirm,
      }: {
        open?: boolean;
        onConfirm: () => void | Promise<void>;
      } = $props();`),
    ).toEqual(['open', 'onConfirm']);
  });

  it('ignores a commented-out prop', () => {
    expect(
      propsFrom(`let {
        title,
        // description, — dropped, nothing rendered it
      } = $props();`),
    ).toEqual(['title']);
  });

  // A snippet or a plain module has no props to document, and that is not a failure.
  it('returns nothing when the source declares no props', () => {
    expect(propsFrom(`const cc = $derived(code.trim());`)).toEqual([]);
  });
});
