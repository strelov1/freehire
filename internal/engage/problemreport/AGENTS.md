# Problem reports

Account-bound operation failures and missing capabilities, reported through OAuth MCP.
This package validates and sanitizes concise summaries, then commits one UTC-day quota
write and an open queue upsert in one transaction. Repeats consume quota and increment
request_count; only the same account and normalized sanitized content deduplicate.
Resolved/dismissed reports do not deduplicate new reports.

The database is private operator data. Never put report summaries, tool arguments,
credentials, or other free text into logs. The post-commit `problem_report_recorded`
event contains only report id, validated category, count, and server request id.
Tool names and supplied error codes stay in the private queue.

Redaction covers recognizable credential assignments, common token formats, private
key blocks, emails, and URL credentials/query strings/fragments. It is a defense in
depth, not a guarantee that arbitrary sensitive prose can be detected. Tool instructions
require summaries without raw conversations, CV content, credentials, or personal details.

Operator triage: [problem-reports.md](../../../docs/agents/problem-reports.md).
