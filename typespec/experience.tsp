import "@typespec/http";

using Http;

namespace Freehire;

// The owner's view of their experience bank: the places (jobs, projects) and the
// achievements recorded under them. An entry's shape belongs to the bank, so entries travel
// as written by it. Provenance is never read from a body: a create is `manual`, and a keyed
// edit rewrites the words and leaves the label alone. With a session or an API key, except
// the merge, which is the session cookie only.

/**
 * How the bank came to hold an achievement. Only the first three are the candidate
 * speaking and may be written into a CV; `agent_inferred` is the assistant's own reading.
 */
enum ExperienceProvenance {
  cv_import,
  stated_in_chat,
  manual,
  agent_inferred,
}

enum ExperienceEmploymentKind {
  job,
  project,
}

/** One place where evidence was produced. A job carries `company`; a project carries its title as `name`. */
model ExperienceEmployment {
  id: string;
  kind: ExperienceEmploymentKind;
  company?: string;
  name?: string;
  role?: string;
  location?: string;
  start?: PeriodDate;
  end?: PeriodDate;
  current?: boolean;
  summary?: string;

  /** Portfolio URL, typically on a project. */
  link?: string;

  stack?: string[];
}

/** One piece of evidence, at the grain of a CV bullet. The three flags below are computed at read time. */
model ExperienceAtom {
  id: string;
  employment_id?: string;
  claim: string;
  context?: string;
  metrics?: string[];
  skills?: string[];
  provenance: ExperienceProvenance;
  source_ref?: string;
  needs_context: boolean;
  needs_metrics: boolean;

  /** Shared id of a soft-duplicate group (its first member), when the list clustered this atom. */
  cluster_id?: string;
}

model ExperienceEmploymentWithAtoms {
  ...ExperienceEmployment;
  atoms: ExperienceAtom[];
}

/** The places with their achievements, and the achievements that belong to no place. */
model ExperienceBank {
  employments: ExperienceEmploymentWithAtoms[];
  unplaced: ExperienceAtom[];
}

model MergeAtoms {
  /** Exactly two achievement ids; anything else is a 400. */
  ids?: string[] | null;
}

@summary("Your employments and their evidence atoms.")
@useAuth(SessionOrKey)
@tag("Experience")
@route("/me/experience")
@get
op listExperience(): Data<ExperienceBank> | Problem;

// The body is a place; its shape is the bank's.
/**
 * An employment carries no claim of its own (only the atoms attached to it do), so
 * nothing about provenance applies here. `kind` is `job` or `project`; a project’s name
 * arrives as `name` on the wire (legacy `company` accepted as a fallback), a job’s as
 * `company`.
 */
@summary("Record a new place — a job or a project.")
@useAuth(SessionOrKey)
@opExample(#{
  parameters: #{
    body: #{
      kind: "job",
      company: "Acme",
      role: "Senior Backend Engineer",
      start: "2023-02",
      current: true,
    },
  },
  returnType: #{
    _: 201,
    body: #{
      data: #{
        id: "3fa8…",
        kind: "job",
        company: "Acme",
        role: "Senior Backend Engineer",
        start: "2023-02",
        current: true,
      },
    },
  },
})
@tag("Experience")
@route("/me/experience/employments")
@post
op addEmployment(@body body: unknown): {
  @statusCode _: 201;
  @body body: Data<unknown>;
} | Problem;

/** The body is a place; blanks are kept. 404 for an id the caller does not own. */
@summary("Edit one employment.")
@useAuth(SessionOrKey)
@opExample(#{
  parameters: #{
    id: "3fa8…",
    body: #{
      company: "Acme",
      role: "Staff Engineer",
    },
  },
  returnType: #{
    data: #{
      id: "3fa8…",
      kind: "job",
      company: "Acme",
      role: "Staff Engineer",
    },
  },
})
@tag("Experience")
@route("/me/experience/employments/{id}")
@put
op updateEmployment(/** The employment id. */ @path id: string, @body body: unknown): Data<unknown> | Problem;

/** 409 for a keyed caller while the place still holds achievements. */
@summary("Remove an employment and its atoms.")
@useAuth(SessionOrKey)
@tag("Experience")
@route("/me/experience/employments/{id}")
@delete
op deleteEmployment(/** The employment id. */ @path id: string): NoContentResponse | Problem;

// The body is an achievement; its provenance is set by the route, whatever the body
// says.
/**
 * The only route besides the assistant’s own tool that can add evidence outside a chat
 * session, which is why `provenance` is forced to `manual` regardless of what the body
 * sends — an authenticated POST with no chat transcript behind it can only honestly be
 * the owner’s own words. When the account has opted into requiring it, an empty `context`
 * is a 400.
 */
@summary("Record a new achievement.")
@useAuth(SessionOrKey)
@opExample(#{
  parameters: #{
    body: #{
      claim: "Cut p99 checkout latency 40%",
      employment_id: "3fa8…",
      metrics: #["40%", "12k rps"],
    },
  },
  returnType: #{
    _: 201,
    body: #{
      data: #{
        id: "88b1…",
        employment_id: "3fa8…",
        claim: "Cut p99 checkout latency 40%",
        metrics: #["40%", "12k rps"],
        provenance: "manual",
      },
    },
  },
})
@tag("Experience")
@route("/me/experience/atoms")
@post
op addAtom(@body body: unknown): {
  @statusCode _: 201;
  @body body: Data<unknown>;
} | Problem;

// Folds two owned achievements into one. 409 when they conflict.
/**
 * Session-only, unlike its sibling create/edit/delete routes — the merge folds the
 * discarded atom’s metrics and skills into the kept one while the kept one’s own
 * provenance stands, letting a `manual` atom absorb numbers a model had inferred, so it
 * cannot be widened to a key the way the others were.
 */
@summary("Fold two of your atoms into one richer keep.")
@opExample(#{
  parameters: #{
    body: #{
      ids: #["88b1…", "91c2…"],
    },
  },
  returnType: #{
    data: #{
      id: "88b1…",
      claim: "Cut p99 checkout latency 40% (12k rps)",
      metrics: #["40%", "12k rps"],
      provenance: "manual",
    },
  },
})
@tag("Experience")
@route("/me/experience/atoms/merge")
@post
op mergeAtoms(@body body: MergeAtoms): Data<unknown> | Problem;

/** A keyed edit leaves the provenance alone. */
@summary("Edit one evidence atom.")
@useAuth(SessionOrKey)
@opExample(#{
  parameters: #{
    id: "88b1…",
    body: #{
      claim: "Cut p99 checkout latency 40% (12k rps)",
    },
  },
  returnType: #{
    data: #{
      id: "88b1…",
      claim: "Cut p99 checkout latency 40% (12k rps)",
      provenance: "manual",
    },
  },
})
@tag("Experience")
@route("/me/experience/atoms/{id}")
@put
op updateAtom(/** The atom id. */ @path id: string, @body body: unknown): Data<unknown> | Problem;

@summary("Remove an evidence atom.")
@useAuth(SessionOrKey)
@tag("Experience")
@route("/me/experience/atoms/{id}")
@delete
op deleteAtom(/** The atom id. */ @path id: string): NoContentResponse | Problem;
