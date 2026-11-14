"""Run OCR without publishing provider diagnostics or private connection settings."""

import argparse
import json
import os
import re
import subprocess
import tempfile
from pathlib import Path
from urllib.parse import urlsplit

MARKER = "<!-- freehire-ocr-review -->"
BACKGROUND = (
    "freehire is an IT job aggregator using Go, Fiber, PostgreSQL, sqlc, Meilisearch "
    "and SvelteKit. Follow AGENTS.md. Report actionable correctness, security and "
    "performance issues with concrete failure scenarios. Write in English. "
    "Never include credentials or gateway addresses."
)


def render_result(
    result: dict, head: str, private_values: list[str]
) -> tuple[str, bool]:
    if not isinstance(result, dict) or not isinstance(
        result.get("comments") or [], list
    ):
        result = {"status": "failed"}
    if any(not isinstance(comment, dict) for comment in result.get("comments") or []):
        result = {"status": "failed"}
    complete = result.get("status") in {
        "complete",
        "success",
        "skipped",
    } and not result.get("warnings")
    comments = result.get("comments") or []
    lines = [MARKER, f"**OpenCodeReview** — commit `{head[:12]}`", ""]
    if not complete:
        lines.append(
            "Review incomplete. Retry with `@codereview`; diagnostics are kept private."
        )
    elif result.get("status") == "skipped":
        lines.append("No reviewable changes.")
    elif not comments:
        lines.append("No findings reported.")
    for comment in comments:
        location = f"{comment.get('path', 'Unpositioned finding')}:{comment.get('start_line', 0)}"
        content = str(comment.get("content", "")).replace("@", "@\u200b")
        location = location.replace("@", "@\u200b")
        lines.extend(
            [
                "",
                f"**{comment.get('severity', 'unknown')}** · `{location}`",
                content,
            ]
        )
    body = "\n".join(lines)
    # Include the hostname so errors cannot reveal it without the full URL.
    values = set(filter(None, private_values))
    for value in list(values):
        if value.startswith(("https://", "http://")):
            parsed = urlsplit(value)
            values.update(
                filter(
                    None,
                    [parsed.hostname, parsed.netloc, parsed.username, parsed.password],
                )
            )
    for value in sorted(values, key=len, reverse=True):
        for needle in {value, value.replace("@", "@\u200b")}:
            body = re.sub(re.escape(needle), "[private]", body, flags=re.IGNORECASE)
    if len(body) > 55000:
        body = (
            body[:55000]
            + "\n\nReview display truncated. Narrow the PR to see all findings."
        )
    return body, complete


def run_review(base: str, head: str, output: Path) -> int:
    failure = {"status": "failed"}
    result = failure
    try:
        if not all(re.fullmatch(r"[0-9a-f]{40}", sha) for sha in [base, head]):
            raise ValueError("Invalid commit SHA")
        if not os.environ.get("OCR_LLM_URL") or not os.environ.get("OCR_LLM_TOKEN"):
            raise ValueError("Missing private connection settings")
        with tempfile.TemporaryDirectory() as folder:
            raw = Path(folder) / "result.json"
            process = subprocess.run(
                [
                    "ocr",
                    "review",
                    "--from",
                    base,
                    "--to",
                    head,
                    "--audience",
                    "agent",
                    "--format",
                    "json",
                    "--concurrency",
                    "2",
                    "--background",
                    BACKGROUND,
                    "--output",
                    str(raw),
                ],
                capture_output=True,
                timeout=40 * 60,
                check=False,
            )
            if raw.is_file():
                result = json.loads(raw.read_text())
            if not isinstance(result, dict):
                raise ValueError("Invalid review result")
            if process.returncode:
                result["status"] = "failed"
    except (OSError, ValueError, TypeError, subprocess.TimeoutExpired):
        result = failure
    body, complete = render_result(
        result,
        head,
        [os.environ.get("OCR_LLM_URL", ""), os.environ.get("OCR_LLM_TOKEN", "")],
    )
    output.write_text(json.dumps({"body": body, "complete": complete}))
    output.chmod(0o600)
    return 0 if complete else 1


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", required=True)
    parser.add_argument("--head", required=True)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    raise SystemExit(run_review(args.base, args.head, args.output))
