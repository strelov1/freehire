"""The two Scrapling-backed fetchers the service serves, and which proxy each egresses through.
Scrapling is imported lazily, so the rest of the service (and its tests) loads without it."""

import itertools
from dataclasses import dataclass, field
from typing import Mapping, Optional
from urllib.parse import urlsplit

from app import FetchError, FetchRequest, FetchResult
from pool import SessionPool

# Headers a caller may not pass through. User-Agent would undo the fetcher's browser
# impersonation (the Go client identifies itself as freehire); the rest are the transport's.
_DROPPED = frozenset({"user-agent", "host", "content-length", "accept-encoding", "connection"})

# Response headers that frame the upstream's bytes. The fetcher returns the body decoded and
# whole, so these would describe bytes the caller never receives.
_FRAMING = frozenset({"content-encoding", "content-length", "transfer-encoding"})

# Statuses an http request through the rotating proxy is retried on: each leaves from another
# exit IP, so a refusal is usually one flagged address.
_REFUSED = frozenset({403, 429})

# A browser session the source refused is retired: a clearance belongs to the exit IP, and the
# next session opens on another one. 405 is how InfoJobs (Adevinta's bot wall, the same one in
# front of ePreselec) refuses: measured 2026-10-30, after ~45 offer pages one session answered
# 405 to 942 of the next ~950 until it retired by age, and the next session was served again.
# It is a refusal for sessions only — the http callers that meet it (adzuna-liveness, epreselec)
# re-send a 405 under their own budgets, and a retry here would multiply each of theirs.
_SESSION_REFUSED = _REFUSED | {405}

# Attempts an http request gets through the rotating proxy when it is refused (403/429) or the
# transport fails. Each leaves from another exit IP, so either is usually one bad address:
# measured 2026-10-20, 1 of 11 Wellfound requests got a 403 the next IP did not, and kontur
# timed out at 40 s on one exit.
ROTATING_ATTEMPTS = 3

HTTP_TIMEOUT_SECONDS = 40
BROWSER_TIMEOUT_MS = 90_000


@dataclass
class ProxyConfig:
    """Which proxy a `proxy: true` request egresses through.

    `rotating` serves the http fetcher: a gateway that hands out a new exit IP per request, so a
    per-IP rate limit sees each request from a different address. `sticky` serves browser
    sessions: each new session takes the next fixed exit IP, so a challenge solved on it stays
    valid for every later page of that session.

    With only SOURCES_PROXY_URL set, that one proxy serves both — the behaviour before rotation.
    """

    rotating: Optional[str]
    sticky: list
    _cycle: object = field(default=None, repr=False)

    def __post_init__(self):
        self._cycle = itertools.cycle(self.sticky) if self.sticky else None

    @property
    def available(self) -> bool:
        return bool(self.rotating or self.sticky)

    def next_sticky(self) -> Optional[str]:
        return next(self._cycle) if self._cycle else self.rotating

    @classmethod
    def from_env(cls, env: Mapping[str, str]) -> "ProxyConfig":
        single = env.get("SOURCES_PROXY_URL", "").strip() or None
        rotating = env.get("SCRAPLING_PROXY_ROTATING", "").strip() or single
        template = env.get("SCRAPLING_PROXY_STICKY", "").strip()
        sticky = [single] if single else []
        if template:
            raw = env.get("SCRAPLING_PROXY_STICKY_COUNT", "").strip()
            if not raw.isdigit() or int(raw) < 1 or "{n}" not in template:
                raise ValueError("SCRAPLING_PROXY_STICKY needs a {n} placeholder and a positive SCRAPLING_PROXY_STICKY_COUNT")
            sticky = [template.replace("{n}", str(n)) for n in range(1, int(raw) + 1)]
        return cls(rotating=rotating, sticky=sticky)


def forwardable_headers(headers: dict) -> dict:
    return {k: v for k, v in headers.items() if k.lower() not in _DROPPED}


def _result(page) -> FetchResult:
    headers = getattr(page, "headers", None) or {}
    return FetchResult(
        status=int(page.status),
        url=str(page.url),
        headers={str(k): str(v) for k, v in dict(headers).items() if str(k).lower() not in _FRAMING},
        body=bytes(page.body),
    )


def _scrapling_send(method: str):
    from scrapling.fetchers import AsyncFetcher

    return {"GET": AsyncFetcher.get, "POST": AsyncFetcher.post, "PUT": AsyncFetcher.put, "DELETE": AsyncFetcher.delete}[method]


def http_fetcher(proxies: ProxyConfig, send=_scrapling_send):
    """One request through curl_cffi with Chrome's TLS fingerprint, through the rotating proxy
    when asked. A refusal or a transport failure through that proxy is retried, since the retry
    leaves from another IP; any other outcome is returned as is — the Go client does not retry
    over this tier."""

    async def fetch(req: FetchRequest) -> FetchResult:
        kwargs = dict(
            impersonate="chrome",
            headers=forwardable_headers(req.headers),
            proxy=proxies.rotating if req.proxy else None,
            timeout=HTTP_TIMEOUT_SECONDS,
            follow_redirects=True,
            retries=1,
        )
        if req.body and req.method in ("POST", "PUT"):
            kwargs["data"] = req.body
        attempts = ROTATING_ATTEMPTS if req.proxy and proxies.rotating else 1
        for attempt in range(1, attempts + 1):
            try:
                result = _result(await send(req.method)(req.url, **kwargs))
            except Exception as e:
                if attempt < attempts:
                    continue  # a slow or dead exit IP; the next attempt leaves from another
                raise FetchError(f"http fetch {req.url}: {type(e).__name__}: {e}") from e
            if result.status not in _REFUSED or attempt == attempts:
                return result
        raise AssertionError("unreachable: the last attempt always returns or raises")

    return fetch


class _Refused(Exception):
    """Carries a refusal out of the pool so the session that met it is discarded."""

    def __init__(self, result: FetchResult):
        self.result = result


def browser_fetcher(pool: SessionPool):
    """A fetcher that navigates on a warm stealth session for (host, proxy) from the pool. A page
    the source refused is still returned — the caller reads the status — but its session goes."""

    async def fetch(req: FetchRequest) -> FetchResult:
        key = (urlsplit(req.url).hostname, req.proxy)

        async def navigate(session):
            result = _result(await session.fetch(req.url))
            if result.status in _SESSION_REFUSED:
                raise _Refused(result)
            return result

        try:
            return await pool.run(key, navigate)
        except _Refused as r:
            return r.result
        except Exception as e:
            raise FetchError(f"browser fetch {req.url}: {type(e).__name__}: {e}") from e

    return fetch


async def _open_stealth(key, proxy: Optional[str], tabs: int = 1):
    from scrapling.fetchers import AsyncStealthySession

    # max_pages is how many tabs the session runs at once; the pool sends it no more uses.
    kwargs = dict(headless=True, solve_cloudflare=True, timeout=BROWSER_TIMEOUT_MS, retries=1, max_pages=tabs)
    if proxy:
        kwargs["proxy"] = proxy
    session = AsyncStealthySession(**kwargs)
    await session.start()
    return session


def stealth_session_factory(proxies: ProxyConfig, open_session=_open_stealth, tabs: int = 1):
    """The pool's factory. A proxied session takes the next sticky exit IP; a direct one none.
    Each session opens with `tabs` tabs, the pool's per_session."""

    async def factory(key):
        _, use_proxy = key
        return await open_session(key, proxies.next_sticky() if use_proxy else None, tabs)

    return factory
