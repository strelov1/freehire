import type { components } from '../generated/api.js';
import type { MentorSlots } from './mentors';

export type BuddySummary = components['schemas']['BuddyCardSummary'];

export type BuddyCompany = components['schemas']['BuddyCompany'];

export type BuddyStatus = components['schemas']['BuddyStatus'];

export type BuddySimilar = components['schemas']['BuddySimilar'];

export type BuddyFeedPage = components['schemas']['BuddyFeedPage'];

export type BuddyProfile = components['schemas']['BuddyProfile'];

/** One member's feed events per UTC day over the last year; only days with an event.
 *  `count` is always applied + interview + offer. */
export type BuddyActivity = components['schemas']['BuddyActivity'];

/** The mock-interview topics, the server's vocabulary (internal/engage/buddy). */
export type BuddyMockTopic = components['schemas']['BuddyMockTopic'];

/** A member's mock-host settings. The link reaches only the two people in a booked mock. */
export type BuddyMockSettings = components['schemas']['BuddyMockSettingsOut'];

/** One member's open mock slots in the viewer's zone, and the topics they host. Never the link. */
export interface BuddyMockSlots extends MentorSlots {
  topics: BuddyMockTopic[];
}

/** One mock as its participant sees it. `counterpart` is null once the other member left or a
 *  block stands between them; `their_review` is null until revealed (both reviewed, or a week
 *  after the end). Instants are absolute (`Z`), so they may be rendered in any zone. */
export type BuddyMock = components['schemas']['BuddyMock'];

export type BuddyMocks = components['schemas']['BuddyMockList'];

/** A member the viewer could book a mock with now: the card every buddy list shows, the
 *  topics they host, their soonest open slot in the viewer's zone (`local_start` already
 *  resolved there) and their revealed-review record. Never the link. */
export type BuddyMockHost = components['schemas']['BuddyMockHost'];

export interface BuddyMockHosts {
  hosts: BuddyMockHost[];
  timezone: string;
}

/** A member page's mock record: revealed reviews only, without their authors. */
export type BuddyMockSummary = components['schemas']['BuddyMockSummary'];

export type ReactionKind = Exclude<components['schemas']['BuddyReactionChoice'], ''>;

/** A feed event's reactions: count per kind (no key for a kind nobody chose) and the viewer's own, "" for none. */
export type BuddyReactions = components['schemas']['BuddyReactions'];

export type BuddyFeedItem = components['schemas']['BuddyFeedEntry'];

type WireCounterpart = components['schemas']['ConversationCounterpart'];

/** The other side of a conversation, drawn by the persona they speak as. The wire says
 *  `reachable` and a nullable `detail` separately; this ties them so a reachable counterpart
 *  has its handle and card, and an unreachable one (left, blocked) has nothing at all, not
 *  even the handle. Nothing here names the account behind the persona
 *  (internal/engage/messaging/AGENTS.md). */
export type Counterpart = Omit<WireCounterpart, 'reachable' | 'detail'> &
  ({ reachable: true; detail: NonNullable<WireCounterpart['detail']> } | { reachable: false; detail: null });

export type Conversation = Omit<components['schemas']['Conversation'], 'counterpart'> & { counterpart: Counterpart };

export type ConversationStatus = Conversation['status_for_me'];

export type ConversationMessage = components['schemas']['ConversationMessage'];

/** A reported conversation for moderators; every participant is named the way the
 *  conversation's kind draws them (a buddy by handle). */
export type ConversationReport = components['schemas']['ConversationReport'];
