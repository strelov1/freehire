import type { CandidateCard } from '../generated/contracts';
import type { MentorSlots } from './mentors';

export interface BuddySummary {
  category: string;
  seniority: string;
  country: string;
}

export interface BuddyCompany {
  slug: string;
  name: string;
}

export interface BuddyStatus {
  member: boolean;
  handle?: string;
  card_empty: boolean;
}

export interface BuddySimilar {
  handle: string;
  card_summary: BuddySummary;
  score: number;
  shared_companies: BuddyCompany[];
}

export interface BuddyFeedPage {
  data: BuddyFeedItem[];
  meta: { next_cursor: string | null };
}

export interface BuddyProfile {
  handle: string;
  card_summary: BuddySummary;
  card: CandidateCard;
  card_empty: boolean;
}

/** One member's feed events per UTC day over the last year; only days with an event. */
/** One member's feed events per UTC day; `count` is always applied + interview + offer. */
export interface BuddyActivity {
  days: { day: string; count: number; applied: number; interview: number; offer: number }[];
}

/** The mock-interview topics, the server's vocabulary (internal/engage/buddy). */
export type BuddyMockTopic = 'coding' | 'system_design' | 'behavioral';

/** A member's mock-host settings. The link reaches only the two people in a booked mock. */
export interface BuddyMockSettings {
  timezone: string;
  meeting_url: string;
  topics: BuddyMockTopic[];
  paused: boolean;
}

/** One member's open mock slots in the viewer's zone, and the topics they host. Never the link. */
export interface BuddyMockSlots extends MentorSlots {
  topics: BuddyMockTopic[];
}

interface BuddyMockReview {
  rating: number;
  comment: string;
}

/** One mock as its participant sees it. `counterpart` is null once the other member left or a
 *  block stands between them; `their_review` is null until revealed (both reviewed, or a week
 *  after the end). Instants are absolute (`Z`), so they may be rendered in any zone. */
export interface BuddyMock {
  id: string;
  role: 'host' | 'guest';
  counterpart: string | null;
  starts_at: string;
  ends_at: string;
  topic: BuddyMockTopic;
  status: 'confirmed' | 'cancelled';
  meeting_url: string | null;
  can_cancel: boolean;
  can_review: boolean;
  my_review: BuddyMockReview | null;
  their_review: BuddyMockReview | null;
}

export interface BuddyMocks {
  upcoming: BuddyMock[];
  past: BuddyMock[];
}

/** A member the viewer could book a mock with now: the card every buddy list shows, the
 *  topics they host, their soonest open slot in the viewer's zone (`local_start` already
 *  resolved there) and their revealed-review record. Never the link. */
export interface BuddyMockHost {
  handle: string;
  card_summary: BuddySummary;
  topics: BuddyMockTopic[];
  next_slot: { starts_at: string; local_start: string; utc_offset: string };
  rating_avg: number;
  rating_count: number;
  completed: number;
}

export interface BuddyMockHosts {
  hosts: BuddyMockHost[];
  timezone: string;
}

/** A member page's mock record: revealed reviews only, without their authors. */
export interface BuddyMockSummary {
  completed: number;
  rating_count: number;
  rating_avg: number;
  recent: { rating: number; comment: string; topic: BuddyMockTopic; day: string }[];
}

export type ReactionKind = 'congrats' | 'luck' | 'rooting' | 'same' | 'insight';

/** A feed event's reactions: count per kind (no key for a kind nobody chose) and the viewer's own, "" for none. */
export interface BuddyReactions {
  counts: Partial<Record<ReactionKind, number>>;
  mine: ReactionKind | '';
}

export interface BuddyFeedItem {
  id: number;
  handle: string;
  card_summary: BuddySummary;
  company: BuddyCompany;
  job_title: string | null;
  job_slug: string | null;
  job_location: string | null;
  reactions: BuddyReactions;
  type: 'applied' | 'interview' | 'offer';
  day: string;
}

/** How a buddy counterpart is drawn: by handle and card, and nothing at all once they are
 *  unreachable (left, blocked) — not even the handle. */
type BuddyCounterpart =
  | { persona: 'buddy'; reachable: true; detail: { handle: string; card_summary: BuddySummary } }
  | { persona: 'buddy'; reachable: false; detail: null };

/** The other side of a conversation, drawn by the persona they speak as. Nothing here names
 *  the account behind the persona (internal/engage/messaging/AGENTS.md). */
export type Counterpart = BuddyCounterpart;

export type ConversationStatus = 'incoming' | 'outgoing' | 'open' | 'closed';

export interface Conversation {
  id: number;
  kind: 'buddy';
  status_for_me: ConversationStatus;
  counterpart: Counterpart;
  unread: number;
  last_message_at: string | null;
  created_at: string;
}

export interface ConversationMessage {
  id: number;
  mine: boolean;
  body: string;
  created_at: string;
}

/** A reported conversation for moderators; every participant is named the way the
 *  conversation's kind draws them (a buddy by handle). */
export interface ConversationReport {
  id: number;
  conversation_id: number;
  kind: Conversation['kind'];
  reason: 'spam' | 'harassment' | 'scam' | 'other';
  note: string;
  created_at: string;
  resolved_at: string | null;
  reporter: string;
  reported: string;
  messages: { sender: string; body: string; created_at: string }[];
}
