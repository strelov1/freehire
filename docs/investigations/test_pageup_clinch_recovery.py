import importlib.util
import io
import json
import os
import subprocess
import sys
import unittest
from contextlib import redirect_stdout
from pathlib import Path
from unittest.mock import patch

spec = importlib.util.spec_from_file_location(
    "recovery", Path(__file__).with_name("pageup-clinch-recovery.py")
)
recovery = importlib.util.module_from_spec(spec)
spec.loader.exec_module(recovery)


class RecoveryTest(unittest.TestCase):
    def setUp(self):
        self.rows = [
            dict(
                provider="pageup",
                board=board,
                company=name,
                region="",
                hub=False,
                tenants={},
            )
            for board, name in recovery.EXPECTED_COMPANIES.items()
        ]

    def test_additions_preserve_company_and_repeat_is_empty(self):
        commands = recovery.additions(self.rows)
        self.assertEqual(len(commands), 38)
        self.assertIn("--company=Mader Group", commands[0])
        for row in list(self.rows):
            self.rows.append(
                dict(row, provider="clinch", board=recovery.DESTINATIONS[row["board"]])
            )
        self.assertEqual(recovery.additions(self.rows), [])

    def test_mixed_existing_targets_and_dynamic_query(self):
        for row in list(self.rows)[::2]:
            self.rows.append(
                dict(
                    row,
                    provider="clinch",
                    board=recovery.DESTINATIONS[row["board"]].upper(),
                )
            )
        self.assertEqual(len(recovery.additions(self.rows)), 19)
        query = recovery.preflight_query()
        for board, host in recovery.DESTINATIONS.items():
            self.assertIn(recovery.sql_literal(board), query)
            self.assertIn(recovery.sql_literal(host), query)
        self.assertEqual(recovery.sql_literal("O'Brien"), "'O''Brien'")
        self.assertTrue(
            set(recovery.DESTINATIONS).isdisjoint({"899", "1023", "1034", "1245"})
        )

    def test_changed_origin_company_aborts(self):
        self.rows[-1]["company"] = "Different employer"
        with self.assertRaises(ValueError):
            recovery.additions(self.rows)

    def test_conflicting_destination_aborts_entire_plan(self):
        self.rows.append(
            dict(
                self.rows[-1],
                provider="clinch",
                board=recovery.DESTINATIONS[self.rows[-1]["board"]],
                company="Other",
            )
        )
        with self.assertRaises(ValueError):
            recovery.additions(self.rows)

    def test_missing_origin_or_unexpected_metadata_aborts(self):
        with self.assertRaises(ValueError):
            recovery.additions(self.rows[:1])
        self.rows[0]["hub"] = True
        with self.assertRaises(ValueError):
            recovery.additions(self.rows)

    def test_dry_run_reads_catalog_without_running_writer(self):
        response = subprocess.CompletedProcess([], 0, json.dumps(self.rows))
        output = io.StringIO()
        with (
            patch.dict(os.environ, DATABASE_URL="postgresql:///hire"),
            patch.object(sys, "argv", ["recovery"]),
            patch.object(recovery.subprocess, "run", return_value=response) as run,
            redirect_stdout(output),
        ):
            recovery.main()
        self.assertEqual(run.call_count, 1)
        self.assertEqual(
            run.call_args.args[0][:3], ["psql", "--dbname", "postgresql:///hire"]
        )
        self.assertIn("BEGIN READ ONLY", run.call_args.args[0][-1])
        self.assertNotIn("--apply", output.getvalue())

    def test_apply_runs_only_the_preflighted_writers(self):
        response = subprocess.CompletedProcess([], 0, json.dumps(self.rows))
        with (
            patch.dict(os.environ, DATABASE_URL="postgresql:///hire"),
            patch.object(sys, "argv", ["recovery", "--apply"]),
            patch.object(recovery.subprocess, "run", return_value=response) as run,
            redirect_stdout(io.StringIO()),
        ):
            recovery.main()
        self.assertEqual(run.call_count, 39)
        for call, command in zip(
            run.call_args_list[1:], recovery.additions(self.rows), strict=True
        ):
            self.assertEqual(call.args[0], command + ["--apply"])
            self.assertTrue(call.kwargs["check"])

    def test_apply_conflict_prevents_every_write(self):
        self.rows.append(
            dict(
                self.rows[-1],
                provider="clinch",
                board=recovery.DESTINATIONS[self.rows[-1]["board"]],
                company="Other",
            )
        )
        response = subprocess.CompletedProcess([], 0, json.dumps(self.rows))
        with (
            patch.dict(os.environ, DATABASE_URL="postgresql:///hire"),
            patch.object(sys, "argv", ["recovery", "--apply"]),
            patch.object(recovery.subprocess, "run", return_value=response) as run,
            self.assertRaises(ValueError),
        ):
            recovery.main()
        self.assertEqual(run.call_count, 1)

    def test_apply_writer_failure_stops_remaining_writes(self):
        response = subprocess.CompletedProcess([], 0, json.dumps(self.rows))
        with (
            patch.dict(os.environ, DATABASE_URL="postgresql:///hire"),
            patch.object(sys, "argv", ["recovery", "--apply"]),
            patch.object(
                recovery.subprocess,
                "run",
                side_effect=[
                    response,
                    subprocess.CalledProcessError(1, ["go"]),
                ],
            ) as run,
            redirect_stdout(io.StringIO()),
            self.assertRaises(subprocess.CalledProcessError),
        ):
            recovery.main()
        self.assertEqual(run.call_count, 2)

    def test_database_error_does_not_print_connection_credentials(self):
        response = subprocess.CompletedProcess([], 2, "", "connection failed")
        with (
            patch.dict(os.environ, DATABASE_URL="postgresql://user:secret@host/hire"),
            patch.object(sys, "argv", ["recovery"]),
            patch.object(recovery.subprocess, "run", return_value=response),
            self.assertRaises(RuntimeError) as caught,
        ):
            recovery.main()
        self.assertNotIn("secret", str(caught.exception))


if __name__ == "__main__":
    unittest.main()
