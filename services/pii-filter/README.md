# pii-filter

Local PII span-detection endpoint that backs `internal/candidate/pii` (the `PII_FILTER_URL` detector).
It serves the **openai/privacy-filter** model (ONNX q4) over CPU and returns spans already
mapped to freehire's Kind vocabulary (`NAME`, `ADDRESS`, `EMAIL`, `PHONE`, `LINK`).

It exists so CV text is de-identified **before** it reaches the LLM gateway: the Go backend
calls `POST /detect`, masks the returned spans, and only then sends the CV onward. It is a
detection endpoint, not on the litellm proxy request path.

## Model

Pull the weights **only** from the official repo (a malicious typosquat has existed):

```
huggingface-cli download openai/privacy-filter \
  config.json tokenizer.json onnx/model_q4.onnx onnx/model_q4.onnx_data \
  --local-dir "$PII_FILTER_MODEL_DIR"
```

`openai/privacy-filter` is Apache-2.0, gpt-oss-derived, a bidirectional token classifier over
a BIOES PII taxonomy. We keep `private_person/address/email/phone/url` and drop
`private_date`, `account_number`, and `secret` (see `detector.py:KIND_MAP`).

## Run

```
pip install -r requirements.txt
PII_FILTER_MODEL_DIR=/path/to/privacy-filter PII_FILTER_ADDR=127.0.0.1:8099 python server.py
curl -s localhost:8099/detect -d '{"text":"Jane Doe jane@doe.com"}'
```

`requirements.txt` pins the three runtime packages to exactly what production runs
(`onnxruntime==1.27.0`, `tokenizers==0.23.1`, `numpy==2.5.1`, read from the production venv
on 2026-11-08, Python 3.12), so a rebuild cannot pull a new major of the model runtime. The
deploy in `freehire-ops` must install the venv from this file, and a bump here is a deliberate
change to what production runs.

## Test

`python -m unittest -v` covers the BIOES→span decode, the windowed scan, and the HTTP
fail-closed checks, with the standard library alone; the `pii-filter` CI job runs exactly
that, with no install step. The ONNX inference is validated by the manual smoke check against
real CVs (weights not in CI).

## Notes

- The model scores at most 4096 tokens per pass. Longer text is scored in overlapping windows,
  so a span past that first window is still returned. Past 32768 tokens the request is 400
  (`text exceeds the scan limit`) — the tail is never dropped and answered as "no PII".
- `POST /detect` requires a `Content-Length` and a JSON object whose `text` is a string. A
  missing or malformed length, a body over 2_000_000 bytes, or a non-string `text` is 400
  (413 when the declared length is over the cap) and is not scored. An unexpected failure is
  a generic 500; the exception is logged, not written into the response.
- Span stitching currently uses argmax + contiguous-merge (spike-validated). The shipped
  Viterbi decoder (`viterbi_calibration.json`) is the accuracy upgrade seam.
- Deployment (systemd unit, weights, health-check, egress) lives in `freehire-ops`.
