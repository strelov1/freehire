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

/** The caller's plan and today's usage, without consuming anything. */
@tag("Plan")
@route("/me/plan")
@get
op getMyPlan(): Data<PlanStanding> | Problem;

/** Recent usage entries, newest first. */
@tag("Plan")
@route("/me/plan/history")
@get
op getMyPlanHistory(): Data<PlanHistoryEntry[]> | Problem;
