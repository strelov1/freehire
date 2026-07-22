"""PII span detection over openai/privacy-filter (ONNX).

decode_spans / KIND_MAP are pure and unit-tested; the Model class wraps onnxruntime and
is exercised by the manual smoke check (heavy weights are not in CI). The endpoint returns
spans already mapped to the freehire Kind vocabulary consumed by internal/pii.
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
            spans.append({"start": cur_start, "end": cur_end, "kind": KIND_MAP[cur_base]})

    for label, (start, end) in zip(labels, offsets):
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


class Model:
    """Lazy-loaded ONNX privacy-filter. Imports the heavy deps only on construction so the
    pure helpers above stay importable without onnxruntime installed."""

    def __init__(self, model_dir: str, max_tokens: int = 4096):
        import json
        import os

        import numpy as np
        import onnxruntime as ort
        from tokenizers import Tokenizer

        self._np = np
        self._max = max_tokens
        cfg = json.load(open(os.path.join(model_dir, "config.json")))
        self._id2label = {int(k): v for k, v in cfg["id2label"].items()}
        self._tok = Tokenizer.from_file(os.path.join(model_dir, "tokenizer.json"))
        self._sess = ort.InferenceSession(
            os.path.join(model_dir, "onnx", "model_q4.onnx"),
            providers=["CPUExecutionProvider"],
        )
        self._inputs = {i.name for i in self._sess.get_inputs()}

    def detect(self, text: str):
        enc = self._tok.encode(text)
        ids = enc.ids[: self._max]
        offsets = enc.offsets[: self._max]
        feed = {"input_ids": self._np.array([ids], dtype=self._np.int64)}
        if "attention_mask" in self._inputs:
            feed["attention_mask"] = self._np.ones((1, len(ids)), dtype=self._np.int64)
        logits = self._sess.run(None, feed)[0][0]
        labels = [self._id2label[int(i)] for i in logits.argmax(-1)]
        return decode_spans(labels, offsets)
