"""PII span detection over openai/privacy-filter (ONNX).

decode_spans / KIND_MAP are pure and unit-tested; the Model class wraps onnxruntime and
is exercised by the manual smoke check (heavy weights are not in CI). The endpoint returns
spans already mapped to the freehire Kind vocabulary consumed by internal/pii.

The model scores at most WINDOW_TOKENS tokens at a time. Longer text is scored in
overlapping windows so a span past the first window is still reported. Past MAX_SCAN_TOKENS
the scan is refused: dropping the tail and answering "no PII" would be a silent miss.
"""

from __future__ import annotations

# privacy-filter emits BIOES labels over these bases; we surface only the ones freehire
# masks. Deliberately absent: private_date (load-bearing for experience timelines),
# account_number and secret (out of the CV-PII scope).
KIND_MAP = {
    "private_person": "NAME",
    "private_address": "ADDRESS",
    "private_email": "EMAIL",
    "private_phone": "PHONE",
    "private_url": "LINK",
}

# The ONNX model is fed at most this many tokens per pass — the length it was given before
# the scan was windowed. Overlap keeps an entity that straddles a cut inside one window.
WINDOW_TOKENS = 4096
WINDOW_OVERLAP = 256
# A bound on total work, not a truncation point. Above it detect raises TextTooLong and the
# caller fails closed instead of returning the spans of a prefix.
MAX_SCAN_TOKENS = 32768


class TextTooLong(Exception):
    def __init__(self, tokens: int):
        self.tokens = tokens
        super().__init__(tokens)


def _base(label: str) -> str | None:
    """The entity base of a BIOES label ('B-private_person' -> 'private_person'); None for 'O'."""
    if "-" not in label:
        return None
    return label.split("-", 1)[1]


def decode_spans(labels, offsets):
    """Stitch per-token BIOES labels into char spans, mapping to freehire kinds and dropping
    any base not in KIND_MAP. Contiguous tokens of the same base merge into one span."""
    spans = []
    cur_base = None
    cur_start = 0
    cur_end = 0

    def flush():
        if cur_base and cur_base in KIND_MAP:
            spans.append(
                {"start": cur_start, "end": cur_end, "kind": KIND_MAP[cur_base]}
            )

    for label, (start, end) in zip(labels, offsets, strict=True):
        base = _base(label)
        if base is None or end == 0:  # 'O' or a special token boundary
            flush()
            cur_base = None
            continue
        if base == cur_base:
            cur_end = end
        else:
            flush()
            cur_base, cur_start, cur_end = base, start, end
    flush()
    return spans


def token_windows(n: int, window: int = WINDOW_TOKENS, overlap: int = WINDOW_OVERLAP):
    """Half-open token ranges covering [0, n). Each range is at most `window` long and
    consecutive ranges share `overlap` tokens, so the tail is never dropped."""
    if n <= 0:
        return
    if window < 1 or overlap < 0 or overlap >= window:
        raise ValueError("window must be positive and larger than overlap")
    start = 0
    step = window - overlap
    while start < n:
        end = min(start + window, n)
        yield start, end
        if end >= n:
            return
        start += step


def merge_spans(spans):
    """Collapse spans the overlap made twice, and join same-kind spans that touch."""
    ordered = sorted(spans, key=lambda span: (span["start"], span["end"], span["kind"]))
    merged = []
    for span in ordered:
        if (
            merged
            and merged[-1]["kind"] == span["kind"]
            and span["start"] <= merged[-1]["end"]
        ):
            merged[-1]["end"] = max(merged[-1]["end"], span["end"])
            continue
        merged.append(
            {"start": span["start"], "end": span["end"], "kind": span["kind"]}
        )
    return merged


def scan_tokens(
    ids,
    offsets,
    infer,
    *,
    window: int = WINDOW_TOKENS,
    overlap: int = WINDOW_OVERLAP,
    max_tokens: int = MAX_SCAN_TOKENS,
):
    """Score every token. `infer` maps one window of ids to BIOES labels.

    Over max_tokens the text is refused before any window is scored, so a caller cannot
    mistake a truncated prefix for a complete scan.
    """
    ids = list(ids)
    offsets = list(offsets)
    if len(ids) != len(offsets):
        raise ValueError("token ids and offsets differ in length")
    if len(ids) > max_tokens:
        raise TextTooLong(len(ids))
    found = []
    for start, end in token_windows(len(ids), window, overlap):
        found.extend(decode_spans(infer(ids[start:end]), offsets[start:end]))
    return merge_spans(found)


class Model:
    """Lazy-loaded ONNX privacy-filter. Imports the heavy deps only on construction so the
    pure helpers above stay importable without onnxruntime installed."""

    def __init__(self, model_dir: str, max_tokens: int = MAX_SCAN_TOKENS):
        import json
        import os

        import numpy as np
        import onnxruntime as ort
        from tokenizers import Tokenizer

        self._np = np
        self._max_tokens = max_tokens
        with open(os.path.join(model_dir, "config.json"), encoding="utf-8") as fh:
            cfg = json.load(fh)
        self._id2label = {int(k): v for k, v in cfg["id2label"].items()}
        self._tok = Tokenizer.from_file(os.path.join(model_dir, "tokenizer.json"))
        self._sess = ort.InferenceSession(
            os.path.join(model_dir, "onnx", "model_q4.onnx"),
            providers=["CPUExecutionProvider"],
        )
        self._inputs = {i.name for i in self._sess.get_inputs()}

    def _infer(self, ids):
        feed = {"input_ids": self._np.array([ids], dtype=self._np.int64)}
        if "attention_mask" in self._inputs:
            feed["attention_mask"] = self._np.ones((1, len(ids)), dtype=self._np.int64)
        logits = self._sess.run(None, feed)[0][0]
        return [self._id2label[int(i)] for i in logits.argmax(-1)]

    def detect(self, text: str):
        enc = self._tok.encode(text)
        return scan_tokens(
            enc.ids, enc.offsets, self._infer, max_tokens=self._max_tokens
        )
