import json
import socket
import socketserver
import threading
import unittest
from unittest.mock import patch

import server
from detector import TextTooLong
from server import (
    MAX_BODY_BYTES,
    REQUEST_TIMEOUT_SECONDS,
    BoundedThreadingHTTPServer,
    ContentLengthError,
    Handler,
    body_too_large,
    parse_content_length,
)


class BodyTooLargeTest(unittest.TestCase):
    def test_within_cap(self):
        self.assertFalse(body_too_large(0))
        self.assertFalse(body_too_large(MAX_BODY_BYTES))

    def test_over_cap(self):
        self.assertTrue(body_too_large(MAX_BODY_BYTES + 1))


class HandlerTimeoutTest(unittest.TestCase):
    def test_handler_sets_a_positive_socket_read_timeout(self):
        # A per-connection deadline (set on the raw socket in setup()), distinct from
        # socketserver's own server-level select() timeout — this is what bounds a client
        # that opens a connection and then trickles, or never sends, its body.
        self.assertEqual(Handler.timeout, REQUEST_TIMEOUT_SECONDS)
        self.assertGreater(Handler.timeout, 0)


class BoundedThreadingHTTPServerTest(unittest.TestCase):
    def _server(self, cap):
        server = BoundedThreadingHTTPServer(
            ("127.0.0.1", 0), Handler, max_concurrent_requests=cap
        )
        self.addCleanup(server.server_close)
        return server

    def test_limits_concurrent_requests(self):
        cap = 2
        server = self._server(cap)
        # Simulate `cap` requests already in flight.
        for _ in range(cap):
            self.assertTrue(server._concurrency.acquire(blocking=False))
        # A (cap + 1)th concurrent request must not be admitted immediately — it should
        # queue in the listen backlog instead of getting its own thread right away.
        self.assertFalse(server._concurrency.acquire(blocking=False))

    def test_process_request_thread_releases_its_slot_on_success(self):
        server = self._server(1)
        server._concurrency.acquire()
        with patch.object(
            socketserver.ThreadingMixIn, "process_request_thread", lambda self, *a: None
        ):
            server.process_request_thread(object(), ("127.0.0.1", 0))
        # The slot freed up, so a next request can be admitted.
        self.assertTrue(server._concurrency.acquire(blocking=False))

    def test_process_request_thread_releases_its_slot_on_error(self):
        server = self._server(1)
        server._concurrency.acquire()

        def boom(self, *a):
            raise RuntimeError("simulated handler failure")

        with patch.object(socketserver.ThreadingMixIn, "process_request_thread", boom):
            with self.assertRaises(RuntimeError):
                server.process_request_thread(object(), ("127.0.0.1", 0))
        # A failed request must not leak its slot.
        self.assertTrue(server._concurrency.acquire(blocking=False))


class ParseContentLengthTest(unittest.TestCase):
    def test_missing_is_not_zero(self):
        for header in (None, "", "   "):
            with self.assertRaises(ContentLengthError) as caught:
                parse_content_length(header)
            self.assertEqual(caught.exception.reason, "missing")

    def test_malformed_is_rejected(self):
        for header in ("-1", "1.5", "0x10", "nope", "12abc", "+12"):
            with self.assertRaises(ContentLengthError) as caught:
                parse_content_length(header)
            self.assertEqual(caught.exception.reason, "malformed", header)

    def test_decimal_length(self):
        self.assertEqual(parse_content_length("0"), 0)
        self.assertEqual(parse_content_length(" 42 "), 42)


class FakeModel:
    def __init__(self):
        self.texts = []
        self.boom = False
        self.too_long = False

    def detect(self, text):
        self.texts.append(text)
        if self.boom:
            raise RuntimeError("tokenizer exploded at /opt/secret/model.onnx")
        if self.too_long:
            raise TextTooLong(99999)
        return [{"start": 0, "end": 4, "kind": "EMAIL"}]


class DetectHTTPTest(unittest.TestCase):
    def setUp(self):
        self.model = FakeModel()
        previous = server.MODEL
        server.MODEL = self.model
        self.httpd = BoundedThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.addCleanup(self._close, previous)
        threading.Thread(target=self.httpd.serve_forever, daemon=True).start()

    def _close(self, previous):
        server.MODEL = previous
        self.httpd.shutdown()
        self.httpd.server_close()

    def _exchange(self, raw):
        port = self.httpd.server_address[1]
        with socket.create_connection(("127.0.0.1", port), timeout=5) as sock:
            sock.settimeout(5)
            sock.sendall(raw)
            sock.shutdown(socket.SHUT_WR)
            data = b""
            while b"\r\n\r\n" not in data:
                chunk = sock.recv(4096)
                if not chunk:
                    break
                data += chunk
            head, _, rest = data.partition(b"\r\n\r\n")
            headers = {}
            for line in head.split(b"\r\n")[1:]:
                if b":" in line:
                    key, value = line.split(b":", 1)
                    headers[key.strip().lower()] = value.strip()
            need = int(headers.get(b"content-length", b"0"))
            while len(rest) < need:
                chunk = sock.recv(4096)
                if not chunk:
                    break
                rest += chunk
            status = int(head.split(b"\r\n", 1)[0].split()[1])
            return status, rest[:need]

    def post(self, body, content_length="auto"):
        if content_length == "auto":
            length_header = f"Content-Length: {len(body)}\r\n"
        elif content_length is None:
            length_header = ""
        else:
            length_header = f"Content-Length: {content_length}\r\n"
        raw = (
            "POST /detect HTTP/1.1\r\n"
            "Host: localhost\r\n"
            "Content-Type: application/json\r\n"
            f"{length_header}"
            "Connection: close\r\n"
            "\r\n"
        ).encode() + body
        return self._exchange(raw)

    def test_empty_body_is_400_and_not_scored(self):
        status, body = self.post(b"", content_length="0")
        self.assertEqual(status, 400)
        self.assertNotIn(b"spans", body)
        self.assertEqual(self.model.texts, [])

    def test_missing_content_length_is_400_and_not_scored(self):
        status, body = self.post(b'{"text":"jane@doe.com"}', content_length=None)
        self.assertEqual(status, 400)
        self.assertNotIn(b"spans", body)
        self.assertEqual(self.model.texts, [])

    def test_negative_content_length_is_400_and_not_read_as_the_body(self):
        status, body = self.post(b'{"text":"jane@doe.com"}', content_length="-1")
        self.assertEqual(status, 400)
        self.assertIn(b"malformed Content-Length", body)
        self.assertEqual(self.model.texts, [])

    def test_oversized_content_length_is_413_and_not_scored(self):
        status, body = self.post(
            b'{"text":"jane@doe.com"}', content_length=str(MAX_BODY_BYTES + 1)
        )
        self.assertEqual(status, 413)
        self.assertNotIn(b"spans", body)
        self.assertEqual(self.model.texts, [])

    def test_malformed_json_is_400(self):
        status, body = self.post(b'{"text":')
        self.assertEqual(status, 400)
        self.assertEqual(json.loads(body), {"error": "malformed request"})
        self.assertNotIn(b'{"text":', body)
        self.assertEqual(self.model.texts, [])

    def test_non_string_text_is_400(self):
        for payload in (b'{"text":1}', b'{"nope":"x"}', b'["text"]', b'{"text":null}'):
            status, body = self.post(payload)
            self.assertEqual(status, 400, payload)
            self.assertEqual(json.loads(body), {"error": "text must be a string"})
        self.assertEqual(self.model.texts, [])

    def test_a_string_text_is_scored(self):
        status, body = self.post(b'{"text":"jane@doe.com"}')
        self.assertEqual(status, 200)
        self.assertEqual(
            json.loads(body),
            {"spans": [{"start": 0, "end": 4, "kind": "EMAIL"}]},
        )
        self.assertEqual(self.model.texts, ["jane@doe.com"])

    def test_text_over_the_scan_cap_is_400(self):
        self.model.too_long = True
        status, body = self.post(b'{"text":"long"}')
        self.assertEqual(status, 400)
        self.assertEqual(json.loads(body), {"error": "text exceeds the scan limit"})
        self.assertNotIn(b"99999", body)

    def test_unexpected_error_is_a_generic_500(self):
        self.model.boom = True
        with self.assertLogs("pii-filter", level="ERROR") as logs:
            status, body = self.post(b'{"text":"boom"}')
        self.assertEqual(status, 500)
        self.assertEqual(json.loads(body), {"error": "internal error"})
        self.assertNotIn(b"model.onnx", body)
        self.assertNotIn(b"RuntimeError", body)
        self.assertNotIn(b"Traceback", body)
        self.assertTrue(any("/opt/secret/model.onnx" in line for line in logs.output))


if __name__ == "__main__":
    unittest.main()
