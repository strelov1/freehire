import contextlib
import io
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

from ci_changes import Scope, classify, read_changes


class ClassificationTests(unittest.TestCase):
    def test_frontend_changes_keep_backend_unit_guards(self):
        for path in ("web/src/lib/api.ts", "design-system/src/Button.svelte"):
            with self.subTest(path=path):
                scope = classify([path])
                self.assertEqual(scope, Scope(backend=False, web=True))
                self.assertEqual(scope.backend_tasks, ("unit",))

    def test_backend_changes_do_not_run_independent_web_checks(self):
        for path in ("internal/job/jobview/view.go", "cmd/ingest/main.go", "migrations/0999_test.sql", "go.mod", "go.sum", ".golangci.yml"):
            with self.subTest(path=path):
                scope = classify([path])
                self.assertEqual(scope, Scope(backend=True, web=False))
                self.assertIn("integration-rest", scope.backend_tasks)
                self.assertIn("unit", scope.backend_tasks)

    def test_plan_contract_requires_both_sides(self):
        self.assertEqual(classify(["internal/ai/plan/plan.go"]), Scope(True, True))

    def test_generated_api_docs_are_checked_by_web(self):
        for path in ("docs/API.md", "docs/API.internal.md"):
            self.assertEqual(classify([path]), Scope(False, True))

    def test_prose_and_extension_changes_keep_unit_guards(self):
        for path in ("README.md", "AGENTS.md", "docs/agents/example.md", "openspec/specs/example/spec.md", "extension/lib/tools/testdata/fill-simple-call.json"):
            with self.subTest(path=path):
                scope = classify([path])
                self.assertEqual(scope, Scope(False, False))
                self.assertEqual(scope.backend_tasks, ("unit",))

    def test_go_files_outside_backend_directories_still_run_backend(self):
        self.assertEqual(classify(["web/example_integration_test.go"]), Scope(True, True))
        self.assertTrue(classify(["docs/example.go"]).backend)

    def test_global_and_unknown_inputs_run_everything(self):
        for path in (".github/workflows/ci.yml", ".github/scripts/ci_changes.py", "scripts/check-deps.mjs", "package.json", "pnpm-lock.yaml", "Dockerfile", "services/new/config.json", "new-area/example.txt"):
            with self.subTest(path=path):
                self.assertEqual(classify([path]), Scope(True, True))
        self.assertEqual(classify([]), Scope(True, True))
        self.assertEqual(classify(None), Scope(True, True))

    def test_mixed_changes_union_the_required_checks(self):
        self.assertEqual(classify(["web/src/lib/api.ts", "internal/job/jobview/view.go"]), Scope(True, True))
        self.assertEqual(classify(["internal/job/jobview/view.go", "README.md"]), Scope(True, False))


class GitChangesTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="freehire-ci-changes-")
        self.addCleanup(self.temp.cleanup)
        self.repo = Path(self.temp.name)
        self.git("init", "-q")
        self.git("config", "user.email", "ci@example.test")
        self.git("config", "user.name", "CI Test")
        self.git("config", "core.hooksPath", "/dev/null")
        self.git("config", "commit.gpgsign", "false")

    def git(self, *args):
        return subprocess.check_output(["git", "-C", str(self.repo), *args], text=True).strip()

    def write(self, path, text):
        target = self.repo / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(text)

    def commit(self):
        self.git("add", "--all")
        self.git("commit", "-qm", "Test fixture")
        return self.git("rev-parse", "HEAD")

    def test_renames_keep_old_and_new_paths(self):
        self.write("web/old.ts", "fixture\n")
        base = self.commit()
        (self.repo / "docs").mkdir()
        self.git("mv", "web/old.ts", "docs/new.md")
        head = self.commit()
        paths = read_changes(base, head, self.repo)
        self.assertEqual(set(paths), {"web/old.ts", "docs/new.md"})
        self.assertEqual(classify(paths), Scope(False, True))

    def test_deleted_backend_file_is_not_lost(self):
        self.write("internal/example/file.go", "package example\n")
        base = self.commit()
        (self.repo / "internal/example/file.go").unlink()
        head = self.commit()
        self.assertEqual(classify(read_changes(base, head, self.repo)), Scope(True, False))

    def test_paths_with_spaces_and_newlines_are_not_split(self):
        self.write("README.md", "fixture\n")
        base = self.commit()
        path = "web/file with a\nnewline.ts"
        self.write(path, "fixture\n")
        head = self.commit()
        self.assertEqual(read_changes(base, head, self.repo), [path])

    def test_initial_missing_and_invalid_bases_fall_back_to_full(self):
        self.write("README.md", "fixture\n")
        head = self.commit()
        for base in ("0" * 40, "f" * 40, "", "--cached"):
            with self.subTest(base=base), contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(classify(read_changes(base, head, self.repo)), Scope(True, True))

    def test_cli_only_emits_static_github_output_fields(self):
        self.write("README.md", "before\n")
        base = self.commit()
        self.write("README.md", "after\n")
        head = self.commit()
        result = subprocess.run([sys.executable, str(Path(__file__).with_name("ci_changes.py")), "--base", base, "--head", head], cwd=self.repo, capture_output=True, text=True, check=True)
        outputs = dict(line.split("=", 1) for line in result.stdout.splitlines())
        self.assertEqual(outputs, {"backend_tasks": json.dumps(["unit"]), "web": "false"})


if __name__ == "__main__":
    unittest.main()
