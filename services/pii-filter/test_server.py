import socketserver
import unittest
from unittest.mock import patch

from server import (
    MAX_BODY_BYTES,
    REQUEST_TIMEOUT_SECONDS,
    BoundedThreadingHTTPServer,
    Handler,
    body_too_large,
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
        with patch.object(socketserver.ThreadingMixIn, "process_request_thread", lambda self, *a: None):
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


if __name__ == "__main__":
    unittest.main()
