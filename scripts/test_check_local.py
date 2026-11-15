"""Behavior fixtures for the local check planner."""

import importlib.util
import subprocess
import unittest
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
                scope, commands = helper.plan([path])
                self.assertIn(path, commands[2])
                self.assertIn(["go", "test", "./..."], commands)
                if scope.backend:
                    self.assertIn(["go", "vet", "-tags=integration", "./..."], commands)
        self.assertTrue(helper.plan(["unknown.xyz"])[0].backend)
        self.assertTrue(helper.plan(None)[0].web)

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
