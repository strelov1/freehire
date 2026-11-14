import unittest

from pr_size import GENERATED, MAX_FILES, MAX_LINES, parse_numstat, problem


class PRSizeTest(unittest.TestCase):
    def test_generated_and_lock_files_are_not_counted(self):
        text = "\0".join(
            [
                "10\t2\tinternal/jobs/service.go",
                "500\t400\tinternal/platform/db/jobs.sql.go",
                "900\t0\tinternal/platform/db/models.go",
                "3000\t3000\tweb/pnpm-lock.yaml",
                "5\t5\tgo.sum",
                "7\t7\tpnpm-lock.yaml",
                "40\t0\tweb/src/lib/generated/api.ts",
                "2000\t100\topenapi/api.openapi.json",
                "700\t20\tweb/static/openapi.yaml",
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

    def test_patterns_are_doublestar_globs_for_ocr(self):
        # OCR matches whole paths and its * stops at "/": a wildcard must sit behind **/ or /**.
        for pattern in GENERATED:
            if "*" in pattern:
                self.assertTrue(
                    pattern.startswith("**/") or pattern.endswith("/**"), pattern
                )


if __name__ == "__main__":
    unittest.main()
