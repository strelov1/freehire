package sources

import "errors"

// ErrBoardGone reports that the PLATFORM itself said this board no longer exists — not that
// the crawl failed to read it. An adapter wraps it when the response is a well-formed page
// whose content is the platform's own "no such board" answer.
//
// It exists because that answer does not always arrive as a 404. Paycom serves
// "Job board does not exist." and ApplicantPro/iSolved Hire serve "This career site has been
// disabled." — both with HTTP 200, so every layer below the adapter reads them as a fine
// response and the adapter fails further along on whatever it could not parse. In prod that
// surfaced in board_health as an XML syntax error and a missing session token: 92 boards
// that had been gone for two months, indistinguishable from an adapter bug.
//
// A wrapped error is still an error — the board's crawl failed and BoardHealth counts the
// failure exactly as before. What the sentinel adds is that the failure is now NAMEABLE, so
// a curator can select these boards instead of reading error prose. Nothing retires a board
// automatically: a platform-wide outage that started serving one of these pages would
// otherwise read as its whole fleet dying at once, and retirement is a decision a
// measurement should inform rather than take.
var ErrBoardGone = errors.New("board no longer exists on the platform")
