import unittest

from detector import KIND_MAP, decode_spans


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


if __name__ == "__main__":
    unittest.main()
