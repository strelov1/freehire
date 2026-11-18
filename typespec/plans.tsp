import "@typespec/http";

using Http;

namespace Freehire;

/** What one tier allows for one feature in a UTC day. */
model PlanTierAllowance {
  /** The daily ceiling; with `unlimited`, the fair-use guard behind it. */
  limit: int64;

  unlimited: boolean;
}

model PlanFeature {
  feature: string;
  free: PlanTierAllowance;
  pro: PlanTierAllowance;
  ultra: PlanTierAllowance;
}

union PlanPriceInterval {
  "month",
  "year",
}

union PlanPriceTier {
  "pro",
  "ultra",
  "api",
  "api_data",
}

model PlanPrice {
  id: string;

  /** The smallest currency unit, as the payment provider stores it. */
  amount_cents: int64;

  currency: string;

  interval: PlanPriceInterval;

  /** The price a new subscriber is sold unless they choose another. */
  default: boolean;

  /** The plan the price buys. */
  tier: PlanPriceTier;
}

model Plans {
  features: PlanFeature[];
  prices: PlanPrice[];

  /** The features whose ceiling refuses today. */
  enforced: string[];
}

/** What each plan allows and what it costs, read from the configuration metering uses. */
@useAuth(NoAuth)
@tag("Plans")
@route("/plans")
@get
op getPlans(): Data<Plans> | Problem;
