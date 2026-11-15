import { tick } from 'svelte';
import { atBottom } from './scrolling';

/** Whether AssistantChat's transcript pane follows the stream, and the scroll that keeps it
 *  at the newest content. The transcript binds its scroll container to `el`. */
export class PaneFollow {
  // The transcript's scroll container.
  el = $state<HTMLDivElement | null>(null);
  // Whether arriving turn events may move the pane. Set from the pane's own scroll
  // events, so scrolling up mid-turn holds position and scrolling back resumes
  // following with no further ceremony.
  stick = $state(true);

  /**
   * Bring the newest content into view.
   *
   * `force` is the difference between a frame that ARRIVED and an act the reader
   * PERFORMED. An arriving frame defers to where they are reading; sending a message,
   * starting a run and opening a conversation do not, because the thing each produces
   * is at the bottom and leaving it off screen hides the result of the act.
   */
  toBottom = async (force = false) => {
    if (!force && !this.stick) return;
    await tick();
    // Asked again after the await: a frame that was allowed to scroll when it arrived
    // must not still scroll if the reader started reading during that tick.
    if (!force && !this.stick) return;
    if (!this.el) return;
    this.el.scrollTop = this.el.scrollHeight;
    // Landing at the bottom is also a decision to follow again. The pane's own scroll
    // event would say so a moment later; saying it here spares the next frame that wait.
    this.stick = true;
  };

  /** The pane moved. Follow the stream only while the reader is at its end. */
  onScroll = () => {
    if (this.el) this.stick = atBottom(this.el);
  };
}
