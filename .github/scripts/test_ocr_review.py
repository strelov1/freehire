import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from ocr_review import render_result, run_review


class OCRReviewTest(unittest.TestCase):
    def test_removes_gateway_identity_from_findings(self):
        result = {
            "status": "complete",
            "comments": [
                {
                    "severity": "high",
                    "path": "worker.go",
                    "start_line": 42,
                    "content": "https://gateway.example/v1 failed; gateway.example:443; secret-token",
                }
            ],
        }
        body, complete = render_result(
            result, "a" * 40, ["https://gateway.example/v1", "secret-token"]
        )
        self.assertTrue(complete)
        self.assertIn("worker.go:42", body)
        self.assertNotIn("gateway.example", body)
        self.assertNotIn("secret-token", body)

    def test_partial_review_keeps_findings_without_claiming_success(self):
        body, complete = render_result(
            {
                "status": "partial",
                "comments": [{"severity": "medium", "content": "A bug"}],
            },
            "a" * 40,
            [],
        )
        self.assertFalse(complete)
        self.assertIn("incomplete", body)
        self.assertIn("A bug", body)
        self.assertNotIn("No findings", body)

    def test_private_values_containing_mentions_are_still_redacted(self):
        secret = "private@token"
        body, complete = render_result(
            {"status": "complete", "comments": [{"path": secret, "content": secret}]},
            "a" * 40,
            [secret],
        )
        self.assertTrue(complete)
        self.assertNotIn(secret, body.replace("\u200b", ""))

    def test_failure_diagnostics_are_never_published(self):
        body, complete = render_result(
            {
                "status": "failed",
                "message": "private diagnostic",
                "warnings": ["private diagnostic"],
            },
            "a" * 40,
            [],
        )
        self.assertFalse(complete)
        self.assertNotIn("private diagnostic", body)

    def test_success_without_findings_is_explicit(self):
        body, complete = render_result(
            {"status": "complete", "comments": []}, "a" * 40, []
        )
        self.assertTrue(complete)
        self.assertIn("No findings", body)

    def test_null_comments_are_an_empty_successful_review(self):
        body, complete = render_result(
            {"status": "complete", "comments": None}, "a" * 40, []
        )
        self.assertTrue(complete)
        self.assertIn("No findings", body)

    def test_unknown_result_is_not_a_success(self):
        _, complete = render_result({}, "a" * 40, [])
        self.assertFalse(complete)

    def test_malformed_comments_are_not_a_success(self):
        _, complete = render_result(
            {"status": "complete", "comments": ["invalid"]}, "a" * 40, []
        )
        self.assertFalse(complete)

    def test_limits_comment_size_and_reports_truncation(self):
        body, _ = render_result(
            {"status": "complete", "comments": [{"content": "x" * 100000}]},
            "a" * 40,
            [],
        )
        self.assertLess(len(body), 60000)
        self.assertIn("truncated", body)

    def test_mentions_in_findings_do_not_notify_others(self):
        body, _ = render_result(
            {"status": "complete", "comments": [{"content": "@someone @codereview"}]},
            "a" * 40,
            [],
        )
        self.assertNotIn("@someone", body)
        self.assertNotIn("@codereview", body)

    def test_process_failure_produces_only_generic_failure(self):
        with tempfile.TemporaryDirectory() as folder:
            output = Path(folder) / "result.json"
            with (
                patch.dict(
                    "os.environ",
                    {
                        "OCR_LLM_URL": "https://gateway.example/v1",
                        "OCR_LLM_TOKEN": "private-token",
                    },
                ),
                patch(
                    "ocr_review.subprocess.run",
                    side_effect=OSError("private diagnostic"),
                ) as run,
            ):
                self.assertEqual(run_review("a" * 40, "b" * 40, output), 1)
                run.assert_called_once()
            result = json.loads(output.read_text())
            self.assertFalse(result["complete"])
            self.assertNotIn("private diagnostic", result["body"])


if __name__ == "__main__":
    unittest.main()
