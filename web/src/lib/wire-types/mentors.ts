import type { components } from '#lib/generated/api.js';

/** One mentor as the PUBLIC routes return them — the directory row and the profile read
 *  are the same wire struct (`mentorResponse` in internal/api/handler/mentorship.go).
 *
 *  Deliberately named, unlike a referral offer: a directory of faceless cards gives a
 *  seeker nothing to choose between.
 *
 *  `meeting_url` is absent here on purpose — it is a live room, so only a booked party
 *  receives it, on their own booking. The owner's and moderator's routes add `status`,
 *  `paused` and (for the owner alone) `meeting_url` to this same struct. */
export type Mentor = components['schemas']['MentorshipMentor'];

/** One offerable hour, carrying three views of the same moment on purpose.
 *
 *  `starts_at`/`ends_at` are the absolute instant — what actually gets booked.
 *  `local_start`/`local_end` are RFC 3339 in the VIEWER's zone — what a person reads.
 *  `utc_offset` is the only thing telling apart the two slots that share a wall-clock
 *  label on the autumn daylight-saving transition; render it, or one October evening
 *  shows "02:00" twice with no way to choose between them.
 *
 *  Never re-derive the local fields from `starts_at` in the browser. The server already
 *  did that arithmetic against its own zone database, and a second answer computed here
 *  differs by exactly one hour twice a year — the kind of wrong that looks right. */
export type MentorSlot = components['schemas']['BuddyMockSlot'];

/** The slot endpoint's answer. `timezone` is the zone the server ACTUALLY used, which may
 *  not be the one asked for: an unrecognised name falls back to UTC. A client that assumes
 *  its own request was honoured cannot tell a correct time from a wrong one. */
export interface MentorSlots {
  slots: MentorSlot[];
  timezone: string;
}

/** One booked session, as a PARTY to it sees it.
 *
 *  Unlike a slot this carries only the absolute instants — there is no `local_start`, so
 *  the viewer's zone is applied in the browser. That is not the trap the slot rule warns
 *  about: an instant ending in `Z` is unambiguous, while a slot's label was already
 *  resolved server-side and re-resolving it is what moves it.
 *
 *  `meeting_url` is present here and absent from the public profile: it is a live room,
 *  and only the two people meeting in it have any business holding the address.
 *  `seeker_email` reaches the MENTOR only — a seeker does not need their own back. */
export type MentorSession = components['schemas']['MentorshipBooking'];

/** A party's sessions, split by the server against ONE clock for the whole list, so a
 *  session starting mid-read cannot land in both halves or in neither. A cancelled
 *  session is PAST whatever its start says: nobody is going to it. */
export type MentorSessions = components['schemas']['MentorshipSessions'];

/** One availability row. Exactly one of `weekday` and `date` is set — the schema enforces
 *  it — so the pair is how a rule says which kind it is.
 *
 *  `weekday` is Go's `time.Weekday`: **0 is SUNDAY**, not Monday. That is the storage
 *  order and it is not the order the calendar draws in; `weekdayOrder` maps between them.
 *
 *  `start`/`end` are `HH:MM`, and `24:00` is a valid end — it is how a mentor is available
 *  until midnight without the row spilling onto the next date.
 *
 *  `closure` marks the trick a raw start/end cannot express: a dated row whose start
 *  equals its end CLOSES that date, beating every other rule on it. Render it as "away",
 *  never as "10:00–10:00". */
export interface MentorAvailabilityRule {
  /** What the delete route needs. A rule rendered without one can be shown and never
   *  removed. */
  id: number;
  weekday?: number | null;
  date?: string | null;
  start: string;
  end: string;
  closure: boolean;
}

/** The four calls an availability editor makes. The mentor cabinet and the buddy mock page
 *  share one editor over two APIs, so the editor takes them rather than importing one. */
export interface AvailabilityApi {
  replaceWeek(rows: { weekday: number; start: string; end: string }[]): Promise<void>;
  reload(): Promise<MentorAvailabilityRule[]>;
  addOverride(date: string, start: string, end: string): Promise<MentorAvailabilityRule>;
  remove(id: number): Promise<void>;
}

/** One labeled span of a mentor's own resolved calendar — booked, busy, free or closed —
 *  computed the same way the public slot engine decides what a seeker may book.
 *
 *  `starts_at`/`ends_at` are absolute instants in UTC; render them in the mentor's own
 *  zone (`MentorCalendar.timezone`), not the viewer's — this is a self-view, and the
 *  mentor's own weekly-hours editor already assumes their own zone. */
export type MentorCalendarInterval = components['schemas']['MentorshipCalendarInterval'];

/** The owner's own resolved calendar for one month: a gapless, non-overlapping partition
 *  of the requested window. `timezone` is the mentor's own IANA zone. */
export interface MentorCalendar {
  intervals: MentorCalendarInterval[];
  timezone: string;
}

/** The mentor's own profile: the public shape plus what only its owner sees. */
export type OwnMentorProfile = components['schemas']['MentorshipOwnMentor'];

/** One profile awaiting a moderator. The moderator's view of a mentor — status and pause,
 *  but no meeting link: deciding whether somebody may mentor does not require the address
 *  of the room they meet in. */
export type PendingMentorProfile = components['schemas']['MentorshipPendingMentor'];

/** What a mentor submits. Whole-object on both create and update: the endpoint takes the
 *  same body either way, and a partial one would clear what it omits. */
export interface MentorProfileInput {
  company_slug: string;
  slug: string;
  name: string;
  headline: string;
  bio: string;
  topics: string[];
  languages: string[];
  timezone: string;
  session_minutes: number;
  buffer_before_minutes: number;
  buffer_after_minutes: number;
  notice_minutes: number;
  horizon_days: number;
  meeting_url: string;
  /** The mentor's own opt-in to publish their account's CV headshot. Off by default —
   *  see `Mentor.show_photo`. */
  show_photo: boolean;
  /** One of the platform's seniority levels, or '' to leave it unset — see
   *  `Mentor.seniority`. Always a string here, unlike the optional wire field, so the
   *  form has a plain value to bind a `<select>` to. */
  seniority: string;
}

/** Best-effort, per-field prefill for the mentor-profile CREATE form, composed from the
 *  candidate's résumé, user profile, account and experience bank. Every field is
 *  independently optional — omitted, not an empty string or array, when its source has
 *  nothing to offer — so seed only the fields that are present and leave the rest at
 *  their ordinary blank defaults. A one-time starting point: apply it once on mount,
 *  then treat every field as an ordinary independent input. */
export type MentorProfileSuggestions = components['schemas']['MentorProfileSuggestions'];
