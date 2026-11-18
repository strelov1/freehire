"""Entry point: wire the service from its environment and serve it.

SCRAPLING_FETCH_TOKEN     bearer token every /fetch request must carry (required)
SCRAPLING_FETCH_ADDR      host:port to listen on (default 127.0.0.1:8098)
SCRAPLING_MAX_BROWSERS    live browser sessions at most (default 3)
SCRAPLING_IDLE_SECONDS    close a session unused this long (default 600)
SCRAPLING_TABS_PER_BROWSER  concurrent pages one browser session serves (default 4)
SCRAPLING_SESSION_MAX_SECONDS  retire a session this old even while in use (default 1800)
SCRAPLING_SESSION_MAX_PAGES    retire a session after this many pages (default 500)
SOURCES_PROXY_URL         the proxy `proxy: true` egresses through (optional)
SCRAPLING_PROXY_ROTATING  a gateway giving a new exit IP per request, for http mode (optional)
SCRAPLING_PROXY_STICKY    a proxy URL template with {n}, one fixed exit IP per n, for browser
                          sessions; SCRAPLING_PROXY_STICKY_COUNT says how many (optional)
"""

import asyncio
import contextlib
import os
import sys

from app import create_app
from engines import ProxyConfig, browser_fetcher, http_fetcher, stealth_session_factory
from pool import SessionPool

MAX_REQUEST_BYTES = (
    1 << 20
)  # a crawler's request: a URL, a few headers, a small POST body
MAX_RESPONSE_BYTES = 64 << 20  # the Go client's own cap (sources.maxResponseBody)
REAP_INTERVAL_SECONDS = 30
# How long a browser request may queue for a session. Below the Go transport's own timeout, so
# a request its caller has abandoned gives up instead of later spending a browser on nothing.
ACQUIRE_TIMEOUT_SECONDS = 120
# The hard limit on one browser page, past the navigation's own 90 s timeout and a challenge
# solve. With the acquire bound it stays under the Go transport's 5-minute timeout, so the
# caller hears the service's own error instead of timing out first.
USE_TIMEOUT_SECONDS = 170
DEFAULT_ADDR = ("127.0.0.1", 8098)


def listen_address(raw: str):
    """host:port to bind. An empty host means loopback, never every interface."""
    raw = raw.strip()
    if not raw:
        return DEFAULT_ADDR
    host, _, port = raw.rpartition(":")
    return (host or DEFAULT_ADDR[0], int(port))


def _positive_int(name: str, default: int) -> int:
    raw = os.environ.get(name, "").strip()
    if not raw:
        return default
    value = int(raw)  # a typo fails startup rather than silently taking the default
    if value < 1:
        raise ValueError(f"{name} must be a positive integer, got {raw!r}")
    return value


def main() -> None:
    token = os.environ.get("SCRAPLING_FETCH_TOKEN", "").strip()
    if not token:
        sys.exit("SCRAPLING_FETCH_TOKEN is required")
    import uvicorn

    host, port = listen_address(os.environ.get("SCRAPLING_FETCH_ADDR", ""))
    proxies = ProxyConfig.from_env(os.environ)
    tabs = _positive_int("SCRAPLING_TABS_PER_BROWSER", 4)
    pool = SessionPool(
        stealth_session_factory(proxies, tabs=tabs),
        max_sessions=_positive_int("SCRAPLING_MAX_BROWSERS", 3),
        idle_ttl=_positive_int("SCRAPLING_IDLE_SECONDS", 600),
        acquire_timeout=ACQUIRE_TIMEOUT_SECONDS,
        per_session=tabs,
        max_age=_positive_int("SCRAPLING_SESSION_MAX_SECONDS", 1800),
        max_uses=_positive_int("SCRAPLING_SESSION_MAX_PAGES", 500),
        use_timeout=USE_TIMEOUT_SECONDS,
    )
    app = create_app(
        token=token,
        proxy_available=proxies.available,
        http_fetch=http_fetcher(proxies),
        browser_fetch=browser_fetcher(pool),
        max_request_bytes=MAX_REQUEST_BYTES,
        max_response_bytes=MAX_RESPONSE_BYTES,
    )

    @contextlib.asynccontextmanager
    async def lifespan(_):
        async def reaper():
            while True:
                await asyncio.sleep(REAP_INTERVAL_SECONDS)
                await pool.reap()

        task = asyncio.create_task(reaper())
        try:
            yield
        finally:
            task.cancel()
            with contextlib.suppress(asyncio.CancelledError):
                await task
            await pool.close_all()

    app.router.lifespan_context = lifespan
    uvicorn.run(app, host=host, port=port, log_level="info", access_log=False)


if __name__ == "__main__":
    main()
