"""Exercise the documented runner APT layouts without changing the host."""

import os
import subprocess
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).with_name("configure-apt-mirrors.sh")


class AptMirrorsTests(unittest.TestCase):
    def run_script(self, root):
        return subprocess.run(
            ["bash", str(SCRIPT)],
            env={**os.environ, "CI_APT_ROOT": str(root)},
            capture_output=True,
            text=True,
            check=False,
        )

    def test_supported_runner_layouts_preserve_sources_and_are_idempotent(self):
        for layout in ("sources.list", "sources.list.d/ubuntu.sources"):
            with self.subTest(layout=layout), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                source = root / layout
                source.parent.mkdir(parents=True, exist_ok=True)
                # The mirror URI is identical in legacy and deb822 sources.
                source_text = "mirror+file:/etc/apt/apt-mirrors.txt\nSigned-By: /keys/ubuntu.gpg\n"
                source.write_text(source_text)
                (root / "apt-mirrors.txt").write_text(
                    "http://azure.archive.ubuntu.com/ubuntu/\tpriority:1\n"
                )
                result = self.run_script(root)
                self.assertEqual(result.returncode, 0, result.stderr)
                mirrors = (root / "apt-mirrors.txt").read_text().splitlines()
                self.assertEqual(
                    mirrors,
                    [
                        "https://mirrors.edge.kernel.org/ubuntu/\tpriority:1",
                        "https://archive.ubuntu.com/ubuntu/\tpriority:2",
                        "https://security.ubuntu.com/ubuntu/\tpriority:3",
                    ],
                )
                self.assertEqual(source.read_text(), source_text)
                config = (root / "apt.conf.d/80-ci-mirrors").read_text()
                self.assertIn('Acquire::https::Timeout "15";', config)
                self.assertNotIn("AllowUnauthenticated", config)
                self.assertEqual(self.run_script(root).returncode, 0)
                self.assertEqual(
                    (root / "apt-mirrors.txt").read_text().splitlines(), mirrors
                )
                self.assertEqual(
                    (root / "apt.conf.d/80-ci-mirrors").read_text(), config
                )

    def test_unexpected_layout_fails_without_rewriting_sources(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / "sources.list"
            source.write_text("deb https://archive.ubuntu.com/ubuntu noble main\n")
            (root / "apt-mirrors.txt").write_text("original\n")
            result = self.run_script(root)
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual((root / "apt-mirrors.txt").read_text(), "original\n")
            self.assertFalse((root / "apt.conf.d/80-ci-mirrors").exists())


if __name__ == "__main__":
    unittest.main()
