import "@typespec/http";

using Http;

namespace Freehire;

// The owner's view of their experience bank: the places (jobs, projects) and the
// achievements recorded under them. An entry's shape belongs to the bank, so entries travel
// as written by it. Provenance is never read from a body: a create is `manual`, and a keyed
// edit rewrites the words and leaves the label alone. With a session or an API key, except
// the merge, which is the session cookie only.

/** The places with their achievements, and the achievements that belong to no place. */
model ExperienceBank {
  /** Each place with an `atoms` array of its achievements. */
  employments: unknown[];

  unplaced: unknown[];
}

model MergeAtoms {
  /** Exactly two achievement ids; anything else is a 400. */
  ids?: string[] | null;
}

@tag("Experience")
@route("/me/experience")
@get
op listExperience(): Data<ExperienceBank> | Problem;

/** The body is a place; its shape is the bank's. */
@tag("Experience")
@route("/me/experience/employments")
@post
op addEmployment(@body body: unknown): {
  @statusCode _: 201;
  @body body: Data<unknown>;
} | Problem;

/** The body is a place; blanks are kept. 404 for an id the caller does not own. */
@tag("Experience")
@route("/me/experience/employments/{id}")
@put
op updateEmployment(@path id: string, @body body: unknown): Data<unknown> | Problem;

/** 409 for a keyed caller while the place still holds achievements. */
@tag("Experience")
@route("/me/experience/employments/{id}")
@delete
op deleteEmployment(@path id: string): NoContentResponse | Problem;

/** The body is an achievement; its provenance is set by the route, whatever the body says. */
@tag("Experience")
@route("/me/experience/atoms")
@post
op addAtom(@body body: unknown): {
  @statusCode _: 201;
  @body body: Data<unknown>;
} | Problem;

/** Folds two owned achievements into one. 409 when they conflict. */
@tag("Experience")
@route("/me/experience/atoms/merge")
@post
op mergeAtoms(@body body: MergeAtoms): Data<unknown> | Problem;

/** A keyed edit leaves the provenance alone. */
@tag("Experience")
@route("/me/experience/atoms/{id}")
@put
op updateAtom(@path id: string, @body body: unknown): Data<unknown> | Problem;

@tag("Experience")
@route("/me/experience/atoms/{id}")
@delete
op deleteAtom(@path id: string): NoContentResponse | Problem;
