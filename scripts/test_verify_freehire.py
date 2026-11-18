"""Regression checks for verification helpers; no services or browser required."""

import os
import shutil
import signal
import subprocess
import tempfile
import time
import unittest
from pathlib import Path

HERE = Path(__file__).resolve().parents[1] / ".agents/skills/verify-freehire"


class HelpersTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.env = {
            **{
                key: value
                for key, value in os.environ.items()
                if not key.startswith("VERIFY_")
            },
            "TESTDB_ADMIN_URL": "postgres://localhost/postgres",
        }
        self.run_cmd("git", "init", "-q")
        self.run_cmd("git", "config", "user.email", "verify@example.com")
        self.run_cmd("git", "config", "user.name", "Verify")
        (self.root / "internal").mkdir()
        (self.root / "internal/main.go").write_text("package main\n")
        (self.root / "go.mod").write_text("module example.com/verify\n")
        self.run_cmd("git", "add", ".")
        self.run_cmd("git", "commit", "-qm", "initial")
        self.prefix = self.root / "stack-functions.sh"
        self.prefix.write_text(
            (HERE / "stack.sh").read_text().split('case "${1:-}" in')[0]
        )

    def run_cmd(self, *args, check=True, env=None):
        return subprocess.run(
            args,
            cwd=self.root,
            env=env or self.env,
            check=check,
            text=True,
            capture_output=True,
        )

    def shell(self, command, **kwargs):
        return self.run_cmd(
            "bash", "-c", 'source "$1"; ' + command, "test", str(self.prefix), **kwargs
        )

    def test_source_freshness_covers_dirty_untracked_and_embedded_files(self):
        result = self.shell("source_fingerprint", check=False)
        self.assertEqual(result.returncode, 0, result.stderr)
        initial = result.stdout
        source = self.root / "internal/main.go"
        source.write_text("package changed\n")
        self.assertNotEqual(initial, self.shell("source_fingerprint").stdout)
        source.write_text("package main\n")
        self.assertEqual(initial, self.shell("source_fingerprint").stdout)
        for path in ("internal/new.go", "internal/template.typ", "go.sum"):
            added = self.root / path
            added.write_text("new input\n")
            self.assertNotEqual(initial, self.shell("source_fingerprint").stdout, path)
            added.unlink()
        source.unlink()
        self.assertNotEqual(initial, self.shell("source_fingerprint").stdout)

    def test_doctor_rejects_source_changes_without_a_new_commit(self):
        setup = 'curl() { return 1; }; alive() { return 1; }; mkdir -p "$STATE"; '
        env = {**self.env, "VERIFY_STATE": str(self.root / "state")}
        self.shell(setup + 'source_fingerprint > "$STATE/source-fingerprint"', env=env)
        healthy = self.shell(setup + "doctor", check=False, env=env)
        self.assertIn(
            "ok    API build inputs match the current checkout", healthy.stdout
        )
        (self.root / "internal/new.go").write_text("package main\n")
        stale = self.shell(setup + "doctor", check=False, env=env)
        self.assertIn("FAIL  API build inputs match the current checkout", stale.stdout)

    def test_default_evidence_is_per_instance(self):
        one = self.shell(
            "print_env", env={**self.env, "VERIFY_API_PORT": "18080"}
        ).stdout
        two = self.shell(
            "print_env", env={**self.env, "VERIFY_API_PORT": "18090"}
        ).stdout

        def evidence(output):
            return next(
                line
                for line in output.splitlines()
                if line.startswith("export VERIFY_EVIDENCE=")
            )

        self.assertNotEqual(evidence(one), evidence(two))

    def test_web_readiness_stops_at_elapsed_deadline(self):
        for directory in ("web", "design-system", "state", "bin"):
            (self.root / directory).mkdir()
        pnpm = self.root / "bin/pnpm"
        pnpm.write_text("#!/bin/sh\nexit 0\n")
        pnpm.chmod(0o755)
        result = self.shell(
            'curl() { printf x >> "$ROOT/calls"; printf 000; }; '
            "sleep() { SECONDS=$((SECONDS + 60)); }; start_web",
            check=False,
            env={
                **self.env,
                "VERIFY_STATE": str(self.root / "state"),
                "PATH": f"{self.root / 'bin'}:{self.env['PATH']}",
            },
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertLessEqual(len((self.root / "calls").read_text()), 2)

    def journey_fixture(self, doctor_exit):
        helpers = self.root / "helpers"
        helpers.mkdir()
        shutil.copy(HERE / "run-journey.sh", helpers)
        stack = helpers / "stack.sh"
        stack.write_text(f"""#!/usr/bin/env bash
if [ "$1" = doctor ]; then exit {doctor_exit}; fi
printf 'export VERIFY_EVIDENCE=%q\\n' "$PWD/evidence"
""")
        stack.chmod(0o755)
        spec = self.root / "sample.spec.ts"
        spec.write_text("// sample\n")
        binary = self.root / "bin"
        binary.mkdir()
        pnpm = binary / "pnpm"
        pnpm.write_text("""#!/usr/bin/env bash
printf '%s\\n' "$*" > "$PWD/../pnpm-called"
""")
        pnpm.chmod(0o755)
        (self.root / "web/e2e/__verify__").mkdir(parents=True)
        (self.root / "web/e2e/__verify__/other.spec.ts").write_text("// other run\n")
        return (
            helpers / "run-journey.sh",
            spec,
            {**self.env, "PATH": f"{binary}:{self.env['PATH']}"},
        )

    def test_journey_refuses_unhealthy_stack(self):
        runner, spec, env = self.journey_fixture(1)
        result = self.run_cmd("bash", str(runner), str(spec), check=False, env=env)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / "pnpm-called").exists())

    def test_journey_keeps_other_runs_and_uses_unique_evidence(self):
        runner, spec, env = self.journey_fixture(0)
        for _ in range(2):
            self.run_cmd("bash", str(runner), str(spec), env=env)
        self.assertTrue((self.root / "web/e2e/__verify__/other.spec.ts").exists())
        self.assertEqual(len(list((self.root / "evidence").glob("sample-*"))), 2)

    def lifecycle_fixture(self, blocked):
        helpers = self.root / "lifecycle"
        helpers.mkdir(exist_ok=True)
        shutil.copy(HERE / "verify.sh", helpers)
        stack = helpers / "stack.sh"
        stack.write_text("""#!/usr/bin/env bash
case "$1" in
  env) printf 'export VERIFY_STATE=%q\\n' "$TEST_STATE" ;;
  up) : ;;
  doctor)
    if [ "$TEST_BLOCKED" = doctor ]; then touch "$TEST_READY"; sleep 60; fi ;;
  down) rm -rf "$VERIFY_STATE"; touch "$TEST_DOWN" ;;
esac
""")
        journey = helpers / "run-journey.sh"
        journey.write_text("""#!/usr/bin/env bash
touch "$TEST_READY"
sleep 60
""")
        for path in (stack, journey):
            path.chmod(0o755)
        binary = self.root / "lifecycle-bin"
        binary.mkdir(exist_ok=True)
        for name in ("go", "pnpm", "psql", "curl", "mise"):
            stub = binary / name
            stub.write_text("#!/bin/sh\nexit 0\n")
            stub.chmod(0o755)
        env = {
            **self.env,
            "PATH": f"{binary}:{self.env['PATH']}",
            "TMPDIR": str(self.root),
            "TEST_STATE": str(self.root / "claimed-state"),
            "TEST_READY": str(self.root / "ready"),
            "TEST_DOWN": str(self.root / "down"),
            "TEST_BLOCKED": blocked,
        }
        process = subprocess.Popen(
            ["bash", str(helpers / "verify.sh"), "sample.spec.ts"],
            cwd=self.root,
            env=env,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
        self.addCleanup(lambda: process.poll() is None and process.kill())
        deadline = time.monotonic() + 10
        while not (self.root / "ready").exists():
            if process.poll() is not None or time.monotonic() > deadline:
                self.fail("lifecycle fixture failed to reach blocked phase")
            time.sleep(0.01)
        return process, helpers / "verify.sh", env

    def test_second_lifecycle_cannot_clean_up_an_owned_instance(self):
        process, runner, env = self.lifecycle_fixture("doctor")
        refused = self.run_cmd(
            "bash", str(runner), "other.spec.ts", env=env, check=False
        )
        self.assertNotEqual(refused.returncode, 0)
        self.assertIsNone(process.poll())
        self.assertFalse((self.root / "down").exists())
        process.send_signal(signal.SIGTERM)
        process.communicate(timeout=5)
        self.assertEqual(process.returncode, 143)

    def test_term_interrupts_doctor_and_journey_and_cleans_up(self):
        for phase in ("doctor", "journey"):
            with self.subTest(phase=phase):
                process, _, _ = self.lifecycle_fixture(phase)
                process.send_signal(signal.SIGTERM)
                process.communicate(timeout=5)
                self.assertEqual(process.returncode, 143)
                self.assertTrue((self.root / "down").exists())
                self.assertFalse((self.root / "claimed-state").exists())
                (self.root / "ready").unlink()
                (self.root / "down").unlink()


if __name__ == "__main__":
    unittest.main()
