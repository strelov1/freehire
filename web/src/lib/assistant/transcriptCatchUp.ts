import { getSession, type InterviewReport } from '#lib/assistant/api.js';
import { chatFromTranscript, type ChatState } from '#lib/assistant/chat.js';

/**
 * Re-reading the open session's stored transcript. The server holds the truth about a turn
 * whose stream the client lost, so this is how a returning client catches up.
 */
export class TranscriptCatchUp {
  private generation = 0;

  invalidate() {
    this.generation++;
  }

  constructor(
    private readonly o: {
      isSupport: boolean;
      activeId: () => string | null;
      /** A turn is streaming: that stream is already the newer truth. */
      streaming: () => boolean;
      show: (chat: ChatState, report?: InterviewReport) => void;
    },
  ) {}

  /** Re-read session `id`'s transcript into the view, if it is still the one shown. */
  reload = async (id: string) => {
    if (this.o.isSupport || id !== this.o.activeId()) return;
    const generation = ++this.generation;
    try {
      const { messages, interview_report } = await getSession(id);
      const next = chatFromTranscript(messages);
      if (generation === this.generation && id === this.o.activeId()) this.o.show(next, interview_report);
    } catch {
      /* the transcript stays as it is; the next visit will catch up */
    }
  };

  /** Catch up when the tab comes back. A phone freezes a backgrounded tab, which breaks the
   *  stream while the turn keeps running on the server — so what is on screen when the user
   *  returns is whatever arrived before the freeze, and the session holds the rest. Nothing
   *  is re-read while a turn is streaming. Returns the listener's removal. */
  listen(): () => void {
    const onVisibility = () => {
      const id = this.o.activeId();
      if (document.visibilityState !== 'visible' || this.o.streaming() || !id) return;
      void this.reload(id);
    };
    document.addEventListener('visibilitychange', onVisibility);
    return () => document.removeEventListener('visibilitychange', onVisibility);
  }
}
