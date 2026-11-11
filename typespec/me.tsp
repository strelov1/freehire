import "@typespec/http";

using Http;

namespace Freehire;

model AgentPermissions {
  /** Whether an agent acting for the account may submit applications. Absent reads as false. */
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
@tag("Account")
@route("/me/agent-permissions")
@get
op getAgentPermissions(): Data<AgentPermissions> | Problem;

/** Session cookie only: an agent must not grant itself the right to apply. */
@tag("Account")
@route("/me/agent-permissions")
@put
op setAgentPermissions(@body body: AgentPermissions): Data<AgentPermissions> | Problem;

/** With a session or an API key. Zeroes when the gateway cannot be read. */
@tag("Account")
@route("/me/usage")
@get
op getMyUsage(): Data<Usage> | Problem;
