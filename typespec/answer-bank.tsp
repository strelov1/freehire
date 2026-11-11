import "@typespec/http";

using Http;

namespace Freehire;

/** An answer to a free-text screening question, saved for the next form that asks it. */
model BankedAnswer {
  id: int64;
  topic: string;
  question: string;
  answer: string;

  /** Who wrote it: the candidate, or an agent on their behalf. */
  provenance: string;

  updated_at: utcDateTime;
}

model SaveBankedAnswer {
  question?: string;
  answer?: string;
}

model Saved {
  saved: boolean;
}

model Deleted {
  deleted: boolean;
}

// The caller's screening-answer bank. With a session or an API key.

@tag("Answer bank")
@route("/me/answer-bank")
@get
op listBankedAnswers(): Data<BankedAnswer[]> | Problem;

/** Saves under the question's topic, replacing the answer already there. */
@tag("Answer bank")
@route("/me/answer-bank")
@put
op saveBankedAnswer(@body body: SaveBankedAnswer): Data<Saved> | Problem;

/** 400 for an id that is not a number, 404 for one the caller does not own. */
@tag("Answer bank")
@route("/me/answer-bank/{id}")
@delete
op deleteBankedAnswer(@path id: string): Data<Deleted> | Problem;
