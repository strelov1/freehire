import unittest

from detector import (
    KIND_MAP,
    MAX_SCAN_TOKENS,
    TextTooLong,
    decode_spans,
    scan_tokens,
    token_windows,
)


class DecodeSpansTest(unittest.TestCase):
    def test_maps_person_and_drops_dates(self):
        # BIOES token labels with their char offsets. "private_date" must be dropped
        # (dates are load-bearing for experience timelines, not PII we mask).
        labels = ["B-private_person", "E-private_person", "O", "S-private_date"]
        offsets = [(0, 4), (5, 12), (13, 15), (16, 26)]
        spans = decode_spans(labels, offsets)
        self.assertEqual(spans, [{"start": 0, "end": 12, "kind": "NAME"}])

    def test_merges_contiguous_same_entity(self):
        labels = ["B-private_url", "I-private_url", "E-private_url"]
        offsets = [(0, 5), (5, 10), (10, 18)]
        spans = decode_spans(labels, offsets)
        self.assertEqual(spans, [{"start": 0, "end": 18, "kind": "LINK"}])

    def test_singleton_entity(self):
        labels = ["O", "S-private_email"]
        offsets = [(0, 3), (4, 20)]
        spans = decode_spans(labels, offsets)
        self.assertEqual(spans, [{"start": 4, "end": 20, "kind": "EMAIL"}])

    def test_kind_map_covers_masked_types_only(self):
        self.assertEqual(KIND_MAP["private_person"], "NAME")
        self.assertEqual(KIND_MAP["private_address"], "ADDRESS")
        self.assertEqual(KIND_MAP["private_email"], "EMAIL")
        self.assertEqual(KIND_MAP["private_phone"], "PHONE")
        self.assertEqual(KIND_MAP["private_url"], "LINK")
        # Out-of-scope categories are intentionally absent (never masked).
        self.assertNotIn("private_date", KIND_MAP)


class ScanTokensTest(unittest.TestCase):
    def test_pii_past_the_old_truncation_point_is_reported(self):
        n = 5000
        pii_at = 4500
        self.assertGreater(pii_at, 4096)
        ids = list(range(n))
        offsets = [(i, i + 1) for i in range(n)]

        def infer(window_ids):
            return [
                "S-private_email" if token == pii_at else "O" for token in window_ids
            ]

        spans = scan_tokens(ids, offsets, infer)
        self.assertEqual(spans, [{"start": pii_at, "end": pii_at + 1, "kind": "EMAIL"}])

    def test_windows_cover_every_token_including_the_tail(self):
        ranges = list(token_windows(5000))
        covered = set()
        for start, end in ranges:
            covered.update(range(start, end))
        self.assertEqual(covered, set(range(5000)))
        self.assertGreater(ranges[-1][1], 4096)
        self.assertGreater(ranges[0][1], ranges[1][0])

    def test_an_entity_in_the_overlap_is_reported_once(self):
        n = 5000
        pii_at = 4000
        ids = list(range(n))
        offsets = [(i, i + 1) for i in range(n)]

        def infer(window_ids):
            return [
                "S-private_phone" if token == pii_at else "O" for token in window_ids
            ]

        spans = scan_tokens(ids, offsets, infer)
        self.assertEqual(spans, [{"start": pii_at, "end": pii_at + 1, "kind": "PHONE"}])

    def test_over_the_scan_cap_is_refused_before_any_window(self):
        calls = []

        def infer(window_ids):
            calls.append(window_ids)
            return ["O"] * len(window_ids)

        with self.assertRaises(TextTooLong):
            scan_tokens(
                list(range(MAX_SCAN_TOKENS + 1)),
                [(0, 1)] * (MAX_SCAN_TOKENS + 1),
                infer,
            )
        self.assertEqual(calls, [])


if __name__ == "__main__":
    unittest.main()
