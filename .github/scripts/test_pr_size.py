import unittest

from pr_size import MAX_FILES, MAX_LINES, parse_numstat, problem


class PRSizeTest(unittest.TestCase):
    def test_generated_and_lock_files_are_not_counted(self):
        text = "\0".join(
            [
                "10\t2\tinternal/jobs/service.go",
                "500\t400\tinternal/platform/db/jobs.sql.go",
                "900\t0\tinternal/platform/db/models.go",
                "3000\t3000\tweb/pnpm-lock.yaml",
                "40\t0\tweb/src/lib/generated/api.ts",
                "-\t-\tweb/static/logo.png",
                "",
            ]
        )
        self.assertEqual(parse_numstat(text), (2, 12))

    def test_paths_with_tabs_keep_their_counts(self):
        self.assertEqual(parse_numstat("3\t1\tdocs/a\tb.md\0"), (1, 4))

    def test_limits_are_inclusive(self):
        self.assertIsNone(problem(MAX_FILES, MAX_LINES))
        self.assertIn("limit", problem(MAX_FILES + 1, 0))
        self.assertIn("limit", problem(1, MAX_LINES + 1))


if __name__ == "__main__":
    unittest.main()
