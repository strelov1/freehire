"""Minimal stdlib HTTP wrapper around the ONNX privacy-filter detector.

POST /detect  {"text": "..."}  -> {"spans": [{"start","end","kind"}, ...]}
GET  /health                   -> {"status": "ok"}

Config via env: PII_FILTER_MODEL_DIR (dir holding config.json, tokenizer.json, onnx/),
PII_FILTER_ADDR (default 127.0.0.1:8099). The model loads once at startup.
"""

import json
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from detector import Model

MODEL = None  # set in main()


class Handler(BaseHTTPRequestHandler):
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
            length = int(self.headers.get("Content-Length", 0))
            req = json.loads(self.rfile.read(length) or b"{}")
            spans = MODEL.detect(req.get("text", ""))
            self._json(200, {"spans": spans})
        except Exception as e:  # fail loud so the Go caller fails closed
            self._json(500, {"error": str(e)})

    def log_message(self, *args):  # silence per-request stderr logging
        pass


def main():
    global MODEL
    model_dir = os.environ["PII_FILTER_MODEL_DIR"]
    host, _, port = os.environ.get("PII_FILTER_ADDR", "127.0.0.1:8099").partition(":")
    MODEL = Model(model_dir)
    ThreadingHTTPServer((host, int(port)), Handler).serve_forever()


if __name__ == "__main__":
    main()
