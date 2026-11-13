import type { Attachment } from 'svelte/attachments';

/**
 * Runs `handler` when an <img> fails to load. Use it as `{@attach onImageError(...)}` instead
 * of `onerror={...}`: for `onerror`/`onload` on an <img>, Svelte's SSR writes an inline
 * `onerror="this.__e=event"` into the HTML to replay an error that lands before hydration, and
 * our nonce-based script-src CSP blocks every inline handler — so each broken image logged a
 * CSP violation. An attachment emits nothing on the server; the replay is replaced by the
 * mount-time check below (a complete image with no intrinsic width already failed).
 */
export function onImageError(handler: (img: HTMLImageElement) => void): Attachment<HTMLImageElement> {
  return (img) => {
    const fail = () => handler(img);
    img.addEventListener('error', fail);
    if (img.complete && img.naturalWidth === 0 && img.getAttribute('src')) fail();
    return () => img.removeEventListener('error', fail);
  };
}
