import "@typespec/http";

using Http;

namespace Freehire;

model AgentPermissions {
  /** Whether an agent acting for the account may submit applications. */
  agents_may_apply: boolean;
}

model AgentPermissionsUpdate {
  /** Absent reads as false. */
  agents_may_apply?: boolean;
}

/** Today's AI use on the account's own model credential. No money: the plan bounds use. */
model Usage {
  requests: int64;
  failed: int64;
  tokens: int64;

  /** The plan day the figures cover, YYYY-MM-DD in UTC. */
  period: string;

  resets_at: utcDateTime;
}

/** Readable with a session or an API key. */
@useAuth(SessionOrKey)
@tag("Account")
@route("/me/agent-permissions")
@get
op getAgentPermissions(): Data<AgentPermissions> | Problem;

/** Session cookie only: an agent must not grant itself the right to apply. */
@tag("Account")
@route("/me/agent-permissions")
@put
op setAgentPermissions(@body body: AgentPermissionsUpdate): Data<AgentPermissions> | Problem;

// With a session or an API key. Zeroes when the gateway cannot be read.
/**
 * Counts and token usage, not cost — the gateway’s dollar figure is a list price against
 * a mixed upstream pool, not what you pay; what you spend is a plan allowance, reported
 * by `/me/plan` over this same day. Never fails for a reason you could act on: no usage
 * yet, or an unreachable gateway, both answer 200 with zeroes.
 */
@summary("Your AI request activity today.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #{
      requests: 42,
      failed: 1,
      tokens: 118000,
      period: "2026-08-31",
      resets_at: utcDateTime.fromISO("2026-09-01T00:00:00Z"),
    },
  },
})
@tag("Account")
@route("/me/usage")
@get
op getMyUsage(): Data<Usage> | Problem;

model SkillPulsePoint {
  /** The ISO week's Monday, YYYY-MM-DD. */
  week_start: string;

  open_count: int64;
}

/** How demand for one of the caller's skills moved. */
model SkillPulse {
  skill: string;
  open_count: int64;

  /** Percent change from the first to the latest week; null without two weeks to compare. */
  change_pct: float64 | null;

  /** Oldest first. */
  series: SkillPulsePoint[];
}

model MarketPulseMeta {
  /** The current ISO week's Monday, YYYY-MM-DD. */
  week_start: string;
}

// Demand for the caller's profile skills, week by week. Session cookie only.
/**
 * One entry per profile skill seen in at least one open job; a skill never seen is
 * omitted rather than reported with a zero. `change_pct` is `null` with fewer than two
 * snapshots, or when the earliest snapshot’s count was zero. A profile with no skills yet
 * answers an empty `data` array, not an error.
 */
@summary("Weekly demand trend for your profile skills.")
@opExample(#{
  returnType: #{
    data: #[
      #{
        skill: "go",
        open_count: 640,
        change_pct: 12.5,
        series: #[
          #{
            week_start: "2026-07-06",
            open_count: 570,
          },
          #{
            week_start: "2026-08-10",
            open_count: 640,
          },
        ],
      },
    ],
    meta: #{
      week_start: "2026-08-10",
    },
  },
})
@tag("Account")
@route("/me/market-pulse")
@get
op getMarketPulse(): {
  data: SkillPulse[];
  meta: MarketPulseMeta;
} | Problem;

model Invite {
  link: string;
  invitees: int64;
  rewarded: int64;
  credit_cents: int64;

  /** The discount an invitee gets. */
  percent_off: int64;
}

model PromoCode {
  code?: string;
}

model PromoDiscount {
  percent_off: int64;
}

/** The caller's invite link and what it has earned. Session cookie only. */
@tag("Promo")
@route("/me/invite")
@get
op getInvite(): Data<Invite> | Problem;

/** What a code would take off, without using it. 404 for a code that cannot be used. */
@tag("Promo")
@route("/me/promo/preview")
@post
op previewPromoCode(@body body: PromoCode): Data<PromoDiscount> | Problem;

/** Uses a code. 409 when the caller already used one. */
@tag("Promo")
@route("/me/promo/redeem")
@post
op redeemPromoCode(@body body: PromoCode): Data<PromoDiscount> | Problem;

/** A prefill for the mentor-profile form, from what the account already knows. */
model MentorProfileSuggestions {
  name?: string;
  headline?: string;
  bio?: string;
  languages?: string[];
  topics?: string[];
  timezone?: string;
  company_slug?: string;
}

@useAuth(SessionOrKey)
@tag("Mentorship")
@route("/me/mentorship/profile/suggestions")
@get
op getMentorProfileSuggestions(): Data<MentorProfileSuggestions> | Problem;
