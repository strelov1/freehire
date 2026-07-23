# pii-filter

Local PII span-detection endpoint that backs `internal/pii` (the `PII_FILTER_URL` detector).
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

## Test

`python -m unittest test_detector` covers the pure BIOES→span decode and the kind map. The
ONNX inference is validated by the manual smoke check against real CVs (weights not in CI).

## Notes

- Span stitching currently uses argmax + contiguous-merge (spike-validated). The shipped
  Viterbi decoder (`viterbi_calibration.json`) is the accuracy upgrade seam.
- Deployment (systemd unit, weights, health-check, egress) lives in `freehire-ops`.
