"""Minimal stdlib HTTP wrapper around the ONNX privacy-filter detector.

POST /detect  {"text": "..."}  -> {"spans": [{"start","end","kind"}, ...]}
GET  /health                   -> {"status": "ok"}

Config via env: PII_FILTER_MODEL_DIR (dir holding config.json, tokenizer.json, onnx/),
PII_FILTER_ADDR (default 127.0.0.1:8099). The model loads once at startup.

A request without a well-formed Content-Length, or whose `text` is not a string, is 400.
It is never scored as an empty document. An unexpected failure is a generic 500; the
exception is logged here and not written into the response.
"""

import json
import logging
import os
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from detector import Model, TextTooLong

MODEL = None  # set in main()

log = logging.getLogger("pii-filter")

# This service is reached only by the trusted internal Go backend (see README/AGENTS.md),
# but it still sits on the CV-processing path and decodes whatever a client sends — these
# bounds are defense-in-depth, not a trust boundary of their own.
MAX_BODY_BYTES = 2_000_000  # generous for any real CV text; bounds a runaway/bad client
REQUEST_TIMEOUT_SECONDS = 30  # per-connection socket read/write deadline
MAX_CONCURRENT_REQUESTS = 32  # ThreadingHTTPServer spawns one thread per connection


class ContentLengthError(Exception):
    def __init__(self, reason: str):
        self.reason = reason
        super().__init__(reason)


def body_too_large(content_length):
    """Whether a declared Content-Length exceeds MAX_BODY_BYTES. A pure predicate so the
    cap itself is unit-testable without a running server."""
    return content_length > MAX_BODY_BYTES


def parse_content_length(header):
    """The declared body size. Missing or non-integer is ContentLengthError — never zero.

    A missing header used to become 0, the handler read nothing, and detect("") answered
    200 with no spans. That is a privacy miss, so it is a 400 instead.
    """
    if header is None or str(header).strip() == "":
        raise ContentLengthError("missing")
    text = str(header).strip()
    if not text.isdigit():
        raise ContentLengthError("malformed")
    try:
        return int(text, 10)
    except ValueError:
        raise ContentLengthError("malformed") from None


class Handler(BaseHTTPRequestHandler):
    # Read by BaseHTTPRequestHandler.setup() indirectly through our own override below —
    # http.server does not apply this automatically, unlike socketserver's single-request
    # handle_request(); a per-connection deadline needs setting on the raw socket.
    timeout = REQUEST_TIMEOUT_SECONDS

    def setup(self):
        self.request.settimeout(self.timeout)
        super().setup()

    def _json(self, code, payload):
        body = json.dumps(payload).encode("utf-8")
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path == "/health":
            self._json(200, {"status": "ok"})
        else:
            self._json(404, {"error": "not found"})

    def do_POST(self):
        if self.path != "/detect":
            self._json(404, {"error": "not found"})
            return
        try:
            length = parse_content_length(self.headers.get("Content-Length"))
        except ContentLengthError as err:
            if err.reason == "missing":
                self._json(400, {"error": "Content-Length is required"})
            else:
                self._json(400, {"error": "malformed Content-Length"})
            return
        if body_too_large(length):
            self._json(413, {"error": "request body too large"})
            return
        try:
            raw = self.rfile.read(length)
            if len(raw) > MAX_BODY_BYTES:
                self._json(413, {"error": "request body too large"})
                return
            payload = json.loads(raw)
            if not isinstance(payload, dict) or not isinstance(
                payload.get("text"), str
            ):
                self._json(400, {"error": "text must be a string"})
                return
            spans = MODEL.detect(payload["text"])
        except TextTooLong:
            self._json(400, {"error": "text exceeds the scan limit"})
            return
        except (json.JSONDecodeError, UnicodeError):
            self._json(400, {"error": "malformed request"})
            return
        except Exception:
            log.exception("detect failed")
            self._json(500, {"error": "internal error"})
            return
        # Observability without leaking PII: log the span COUNT only, never the text.
        print(f"detect: {len(spans)} spans", file=sys.stderr, flush=True)
        self._json(200, {"spans": spans})

    def log_message(
        self, *args
    ):  # suppress the default request-line noise (incl. /health)
        pass


class BoundedThreadingHTTPServer(ThreadingHTTPServer):
    """ThreadingHTTPServer spawns one unbounded thread per accepted connection — a slow or
    numerous-enough client can exhaust threads/memory with no defense-in-depth bound. A
    semaphore caps how many requests are handled concurrently: process_request (called
    serially from the accept loop) blocks once the cap is reached, so excess connections
    queue in the listen backlog instead of each getting their own thread immediately."""

    def __init__(
        self, *args, max_concurrent_requests=MAX_CONCURRENT_REQUESTS, **kwargs
    ):
        self._concurrency = threading.BoundedSemaphore(max_concurrent_requests)
        super().__init__(*args, **kwargs)

    def process_request(self, request, client_address):
        self._concurrency.acquire()
        super().process_request(request, client_address)

    def process_request_thread(self, request, client_address):
        try:
            super().process_request_thread(request, client_address)
        finally:
            self._concurrency.release()


def main():
    global MODEL
    model_dir = os.environ["PII_FILTER_MODEL_DIR"]
    host, _, port = os.environ.get("PII_FILTER_ADDR", "127.0.0.1:8099").partition(":")
    MODEL = Model(model_dir)
    BoundedThreadingHTTPServer((host, int(port)), Handler).serve_forever()


if __name__ == "__main__":
    main()
