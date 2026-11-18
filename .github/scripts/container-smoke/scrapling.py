"""Runs only in the network-disabled Scrapling image, never on the runner host."""

import asyncio
import json
import os
import sys
import threading
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

assert os.getuid() == 10001
assert not os.access("/service", os.W_OK)
assert not os.access("/opt/ms-playwright", os.W_OK)
Path(os.environ["HOME"], "smoke-home").write_text("dummy")
cache = Path(os.environ["XDG_CACHE_HOME"])
cache.mkdir(exist_ok=True)
(cache / "smoke-cache").write_text("dummy")
assert json.load(urllib.request.urlopen("http://127.0.0.1:8098/health", timeout=3)) == {
    "status": "ok"
}
try:
    urllib.request.urlopen(
        urllib.request.Request("http://127.0.0.1:8098/fetch", data=b"{}"), timeout=3
    )
    raise AssertionError("missing token must be refused")
except urllib.error.HTTPError as error:
    assert error.code == 401


class Page(BaseHTTPRequestHandler):
    def do_GET(self):
        content = b"<html><head><title>Offline smoke</title></head><body>browser-ready</body></html>"
        self.send_response(200)
        self.send_header("Content-Type", "text/html")
        self.send_header("Content-Length", str(len(content)))
        self.end_headers()
        self.wfile.write(content)

    def log_message(self, *_):
        pass


server = ThreadingHTTPServer(("127.0.0.1", 18080), Page)
threading.Thread(target=server.serve_forever, daemon=True).start()
sys.path.insert(0, "/service")


async def browser_smoke():
    from engines import _open_stealth

    # The real service session factory resolves and launches the relocated browser.
    # Direct session use avoids weakening the service's internal-address refusal.
    session = await _open_stealth(("127.0.0.1", False), None)
    try:
        page = await session.fetch("http://127.0.0.1:18080/")
        assert page.status == 200
        assert b"browser-ready" in bytes(page.body)
    finally:
        await session.close()


try:
    asyncio.run(browser_smoke())
    print(
        "PASS: non-root Scrapling startup, auth refusal, writable HOME/cache and offline browser session"
    )
finally:
    server.shutdown()
    server.server_close()
