/** Stable token the backend embeds in ErrListCap tool/HTTP errors. */
const BULLET_CAP_CODE = 'bullet_cap';

/**
 * Candidate-facing copy when a cv_edit was refused because a role is at the
 * bullet ceiling. Safe to show in the UI: no internal prefixes, reassures that
 * nothing was deleted.
 */
export function bulletCapUserMessage(toolResult: string | undefined): string | null {
  if (!toolResult || !toolResult.includes(BULLET_CAP_CODE)) return null;
  let raw = toolResult.trim();
  try {
    const parsed = JSON.parse(raw) as { error?: unknown };
    if (typeof parsed.error === 'string') raw = parsed.error;
  } catch {
    /* plain string */
  }
  const marked = raw.indexOf(`${BULLET_CAP_CODE}:`);
  if (marked < 0) return null;
  let msg = raw.slice(marked + BULLET_CAP_CODE.length + 1).trim();
  const cut = msg.indexOf('. Set an existing');
  if (cut >= 0) msg = msg.slice(0, cut);
  if (!msg) {
    return 'This role already has the maximum number of bullet points. The edit was not applied. Your existing bullets were kept.';
  }
  if (!msg.endsWith('.')) msg += '.';
  if (!/your existing bullets were kept/i.test(msg)) {
    msg += ' Your existing bullets were kept.';
  }
  return msg;
}
