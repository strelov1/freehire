# Triage account problem reports

OAuth account MCP exposes `report_problem` for operations an agent could not complete.
It records a private review request; a receipt promises neither a fix nor a reply.
There is no public read endpoint. Apply migration `0234_problem_reports.sql` before
serving the tool.

Each report contains the intended goal, expected result, actual failure, category, and
optional tool name/error code. The service removes recognizable secrets, emails, and
URL credentials, query strings, and fragments. Agents must still send concise summaries
without transcripts, CVs, credentials, or personal contact details; automated redaction
cannot identify all sensitive prose.

Use an authorized database session to review the queue:

```sql
SELECT id, user_id, category, goal, expected, actual, tool_name, error_code,
       request_count, first_reported_at, last_reported_at, request_id
FROM problem_reports
WHERE status = 'open'
ORDER BY last_reported_at DESC
LIMIT 100;
```

Find the structured `problem_report_recorded` log event by `report_id`, or use its
server-generated `request_id` to inspect diagnostics for the submission. The request id
belongs to the report submission, not necessarily the earlier operation that failed.
Repeated reports update the queue's request id to the latest submission; earlier ids
remain in the structured events. Logs deliberately omit all summaries and caller-supplied
tool names/error codes. Diagnosing an earlier failure may require its existing tool
error diagnostics; never request tokens or raw CV contents to reproduce it.

Resolve or dismiss after review, with an operator-authored note:

```sql
UPDATE problem_reports
SET status = 'resolved', resolution_note = 'Fixed in the linked change'
WHERE id = 123 AND status = 'open';
```

Use `dismissed` for an issue that will not be acted on. Reports with matching normalized
sanitized contents from the same account share one open row and increment its count.
A report after resolution/dismissal starts a new open row. Each write atomically
creates the report or increments its repeat count. Request frequency policy is
managed by the external gateway in freehire-ops. Account deletion removes reports
and any historical daily quota rows.
