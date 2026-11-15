import "@typespec/http";

using Http;

namespace Freehire;

/** One finished bulk snapshot of the catalogue. */
model CatalogueSnapshot {
  /** UTC day the snapshot is named by, `YYYY-MM-DD`. */
  date: string;

  /** Version of the line shape in the file. A removed or re-meant field bumps it; a new field does not. */
  schema_version: int32;

  /** Number of lines (jobs) in the file. */
  rows: int64;

  /** Size of the gzip file in bytes. */
  bytes: int64;

  /** Hex SHA-256 of the gzip file, to check a download. */
  sha256: string;

  generated_at: utcDateTime;

  /** Pre-signed link to the gzip file; it works without credentials until `url_expires_at`. */
  download_url: string;

  url_expires_at: utcDateTime;
}

// The snapshots the Data plan can download.
/**
 * Lists the finished snapshots, newest first, each with a short-lived pre-signed download URL
 * (about 15 minutes; call this again for a fresh link). Needs the Data plan: any other
 * account gets `403`. `501` means bulk files are not set up on this deployment.
 */
@summary("List bulk catalogue snapshots and their download links.")
@useAuth(SessionOrKey)
@opExample(#{
  returnType: #{
    data: #[
      #{
        date: "2026-11-13",
        schema_version: 1,
        rows: 4500000,
        bytes: 7000000000,
        sha256: "3b1f0c7e5d9a4c2e8f6a1b0d7c3e9f52a4b8d6c1e0f7a3b5c9d2e4f6a8b0c1d3",
        generated_at: utcDateTime.fromISO("2026-11-13T04:10:00Z"),
        download_url: "https://example-bucket.example.com/catalogue/2026-11-13/jobs.jsonl.gz?X-Amz-Signature=...",
        url_expires_at: utcDateTime.fromISO("2026-11-13T12:15:00Z"),
      }
    ],
  },
})
@tag("Catalogue export")
@route("/catalogue/snapshots")
@get
op listCatalogueSnapshots(): Data<CatalogueSnapshot[]> | Problem;
