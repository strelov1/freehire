"""Behavior fixtures for the local check planner."""

import importlib.util
import io
import subprocess
import unittest
from contextlib import redirect_stdout
from pathlib import Path
from unittest.mock import patch

spec = importlib.util.spec_from_file_location(
    "check_local", Path(__file__).with_name("check-local.py")
)
helper = importlib.util.module_from_spec(spec)
spec.loader.exec_module(helper)


class LocalChecks(unittest.TestCase):
    def test_fixtures(self):
        for path in (
            "internal/api/handler/signature.go",
            "migrations/999.sql",
            "typespec/main.tsp",
            "web/src/Page.svelte",
            "scripts/run.sh",
            "docs/example.md",
        ):
            with self.subTest(path=path):
                scope, commands = helper.plan([path], "selected-base")
                self.assertIn(path, commands[2])
                self.assertIn(["go", "test", "./..."], commands)
                if scope.backend:
                    self.assertIn(["go", "vet", "-tags=integration", "./..."], commands)
        self.assertTrue(helper.plan(["unknown.xyz"], "selected-base")[0].backend)
        self.assertTrue(helper.plan(None, "selected-base")[0].web)

    def test_committed_go_checks_use_selected_base(self):
        _, commands = helper.plan(["internal/example.go"], "abc123")
        lint = next(command for command in commands if command[0] == "golangci-lint")
        self.assertIn("--new-from-rev=abc123", lint)

    def test_dirty_secret_inputs_are_explicitly_unscanned(self):
        output = io.StringIO()
        with (
            patch.object(helper.sys, "argv", ["check-local.py", "--base", "main"]),
            patch.object(
                helper,
                "changes",
                return_value=("abc", ["unstaged.env", "untracked.env"]),
            ),
            patch.object(
                helper, "git", return_value=b" M unstaged.env\n?? untracked.env\n"
            ),
            redirect_stdout(output),
        ):
            self.assertEqual(helper.main(), 0)
        self.assertIn("staged diff only", output.getvalue())
        self.assertIn("NOT scanned for secrets", output.getvalue())

    def test_change_sources_and_rename_endpoints(self):
        with patch.object(
            helper,
            "git",
            side_effect=[
                b"abc\n",
                b"old.go\0new.go\0",
                b"unstaged.sh\0",
                b"staged.sql\0",
                b"untracked.svelte\0",
            ],
        ) as git:
            base, paths = helper.changes("main")
        self.assertEqual(base, "abc")
        self.assertEqual(len(paths), 5)
        self.assertIn("old.go", paths)
        self.assertIn("new.go", paths)
        self.assertIn("--no-renames", git.call_args_list[1].args)

    def test_failures_propagate_and_remaining_checks_run(self):
        with patch.object(
            helper.subprocess,
            "run",
            side_effect=[
                subprocess.CompletedProcess([], 1),
                subprocess.CompletedProcess([], 0),
            ],
        ) as run:
            self.assertEqual(helper.execute([["first"], ["second"]]), 1)
            self.assertEqual(run.call_count, 2)


if __name__ == "__main__":
    unittest.main()
