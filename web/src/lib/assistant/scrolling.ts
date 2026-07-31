// When the transcript follows the stream and when it yields to the reader.
//
// The pane scrolls itself to the newest content only while the reader is already at
// the bottom. Scroll up during a turn and the position holds until you come back —
// which is the whole point: an answer long enough to be worth re-reading is exactly
// the answer that used to be impossible to re-read.

/** The distance from the end of the pane that still counts as "at the bottom".
 *
 *  It is not slack for its own sake. A streaming answer appends to its own final
 *  line, so the pane grows taller under a reader who has not moved: measured
 *  exactly, following would switch itself off on its own content, one frame after
 *  it started. The tolerance is roughly a line and a half — large enough to survive
 *  that growth, small enough that a deliberate scroll of any size leaves it. */
export const BOTTOM_TOLERANCE_PX = 64;

/** The three numbers a scrollable element reports about itself. Taken as a plain
 *  shape rather than as an `Element` so the decision can be tested without a DOM. */
export type ScrollMetrics = {
  scrollHeight: number;
  scrollTop: number;
  clientHeight: number;
};

/** Whether the pane should keep following the stream.
 *
 *  The subtraction can go negative — a pane shorter than its viewport, and momentum
 *  scrolling on macOS and iOS, both report a position past the real maximum — and
 *  every negative case is "at the bottom", which `<=` already gives us. */
export function atBottom(m: ScrollMetrics, tolerance = BOTTOM_TOLERANCE_PX): boolean {
  return m.scrollHeight - m.scrollTop - m.clientHeight <= tolerance;
}
