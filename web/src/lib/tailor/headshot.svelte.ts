import { api } from '#lib/api.js';

/**
 * The member's headshot, for the templates that print one. Read once rather than inside the
 * preview: the photo belongs to the profile, not to the CV being edited, and the same URL
 * feeds every template switch. Null while unknown, absent, or unconfigured — the preview then
 * draws the placeholder, exactly as the PDF does. Construct during component initialisation.
 */
export class Headshot {
  src = $state<string | null>(null);

  constructor() {
    $effect(() => {
      let cancelled = false;
      void (async () => {
        try {
          const meta = await api.getPhoto();
          if (cancelled || !meta.present) return;
          this.src = `/api/v1/me/photo/image?v=${encodeURIComponent(meta.uploaded_at ?? '')}`;
        } catch {
          // Best-effort: without it the preview shows the placeholder.
        }
      })();
      return () => {
        cancelled = true;
      };
    });
  }
}
