"""Behavior fixtures for scoped source search."""

import subprocess
import tempfile
import unittest
from pathlib import Path

HELPER = Path(__file__).with_name("source-search.py").resolve()


class SourceSearchTest(unittest.TestCase):
    def test_scope_inputs_and_escape_hatch(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for name in (
                "queries/jobs.sql",
                "migrations/001.sql",
                "typespec/main.tsp",
                "apigen/generated.go",
                "history/old.go",
                "openspec/changes/archive/old/spec.md",
                "AGENTS.md",
            ):
                path = root / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text("fixture_marker\n")

            def search(*flags):
                return subprocess.run(
                    ["python3", str(HELPER), *flags, directory, "fixture_marker"],
                    capture_output=True,
                    text=True,
                )

            result = search()
            self.assertEqual(result.returncode, 0, result.stderr)
            for name in ("jobs.sql", "001.sql", "main.tsp", "old.go"):
                self.assertIn(name, result.stdout)
            for name in ("generated.go", "spec.md", "AGENTS.md"):
                self.assertNotIn(name, result.stdout)
                self.assertIn(name, search("--full").stdout)
            missing = subprocess.run(
                ["python3", str(HELPER), directory, "absent"], capture_output=True
            )
            self.assertEqual(missing.returncode, 1)


if __name__ == "__main__":
    unittest.main()
