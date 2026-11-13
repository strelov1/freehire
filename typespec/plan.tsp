import "@typespec/http";

using Http;

namespace Freehire;

/** Usage against a limit for one metered feature today. */
model PlanAllowance {
  feature: string;
  used: int64;

  /** Absent for an unlimited caller. */
  limit?: int64;

  unlimited: boolean;
  enforced: boolean;
  resets_at: utcDateTime;
}

/** The tiers a caller can be on. */
union PlanTier {
  "free",
  "pro",
  "ultra",
}

/** Where a paid plan was bought. */
union PlanSource {
  "stripe",
  "revenuecat",
  "granted",
}

model PlanStanding {
  plan: PlanTier;

  resets_at: utcDateTime;

  /** When the paid plan lapses, absent on the free plan. */
  pro_until?: utcDateTime;

  /** Where the plan was bought. Absent on the free plan. */
  pro_source?: PlanSource;

  allowances: PlanAllowance[];
}

model PlanHistoryEntry {
  feature: string;
  day: string;
  kind: string;
  label: string;
  subtitle?: string;
  created_at: utcDateTime;
}

// The caller's plan and today's usage, without consuming anything.
/**
 * Which plan you are on and, for every metered AI feature, how much of today you have
 * used against what the day allows. Metered features need a paid plan: the free plan's
 * allowance is zero. The amount resets at `resets_at`; read `unlimited` per allowance, since
 * a paid plan can still have a finite limit for a feature such as `auto-apply`. `enforced`
 * says whether that ceiling turns anybody away yet — while
 * it is `false` a spent allowance is counted and the action still runs, so do not refuse
 * on `used >= limit` alone. Never runs the LLM. A pro caller also carries `pro_until` and
 * `pro_source` — `stripe`, `revenuecat` or `granted` — both absent on the free plan and
 * on a plan that has ended. `pro_source` is behavioural: a client offering an in-app
 * purchase must not offer one to a `stripe` subscriber, who would be charged twice for
 * one plan, and a `revenuecat` subscriber must be sent to their store to cancel rather
 * than to a web page. A tie resolves in the order `stripe`, `revenuecat`, `granted`.
 */
@summary("Your plan and what it allows today.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #{
      plan: "free",
      resets_at: utcDateTime.fromISO("2026-09-01T00:00:00Z"),
      allowances: #[
        #{
          feature: "tailor",
          used: 0,
          limit: 0,
          unlimited: false,
          enforced: true,
          resets_at: utcDateTime.fromISO("2026-09-01T00:00:00Z"),
        },
        #{
          feature: "match",
          used: 0,
          limit: 0,
          unlimited: false,
          enforced: true,
          resets_at: utcDateTime.fromISO("2026-09-01T00:00:00Z"),
        },
      ],
    },
  },
})
@tag("Plan")
@route("/me/plan")
@get
op getMyPlan(): Data<PlanStanding> | Problem;

// Recent usage entries, newest first.
/**
 * Each entry is labelled with what it bought — the job an analysis was run on, the
 * vacancy a CV editing session was opened for — rather than an opaque reference. A
 * `release` entry is one that was given back because the work produced nothing.
 */
@summary("What you spent your allowances on, newest first.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #[
      #{
        feature: "match",
        day: "2026-08-31",
        kind: "consume",
        label: "Job analysis",
        subtitle: "Senior Backend Engineer at Acme",
        created_at: utcDateTime.fromISO("2026-08-31T09:00:00Z"),
      },
    ],
  },
})
@tag("Plan")
@route("/me/plan/history")
@get
op getMyPlanHistory(): Data<PlanHistoryEntry[]> | Problem;
