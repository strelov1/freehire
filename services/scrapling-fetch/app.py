"""HTTP layer of the Scrapling fetch service: request validation, auth and the wire shape.

POST /fetch  {"url", "method"?, "headers"?, "body"? (base64), "mode": "http"|"browser", "proxy"?}
          -> 200 {"status", "url", "headers", "body" (base64)}   the upstream answer, as data
          -> 4xx {"error"}                                       a bad request; nothing fetched
          -> 502 {"error"}                                       the fetch itself failed
GET  /health -> {"status": "ok"}

The fetchers are injected, so this module never imports Scrapling and its tests need no browser.
"""

import asyncio
import base64
import binascii
import hmac
import ipaddress
import json
import logging
import socket
from collections.abc import Awaitable, Callable
from dataclasses import dataclass, field, replace
from urllib.parse import urljoin, urlsplit

from starlette.applications import Starlette
from starlette.requests import Request
from starlette.responses import JSONResponse
from starlette.routing import Route

MODES = ("http", "browser")
# The methods Scrapling's HTTP fetcher implements.
HTTP_METHODS = ("GET", "POST", "PUT", "DELETE")
# Go's default client stops after ten hops. Past that the next target is not requested.
MAX_REDIRECTS = 10
REDIRECT_STATUSES = frozenset({301, 302, 303, 307, 308})
# 301/302/303 historically become GET and drop the body (curl and net/http both do).
# 307/308 keep the method and body.
_GET_ON_REDIRECT = frozenset({301, 302, 303})

log = logging.getLogger("scrapling-fetch")


@dataclass
class FetchRequest:
    url: str
    method: str = "GET"
    headers: dict = field(default_factory=dict)
    body: bytes = b""
    mode: str = "http"
    proxy: bool = False


@dataclass
class FetchResult:
    status: int
    url: str
    headers: dict
    body: bytes


class FetchError(Exception):
    """The fetch could not complete: the service's failure, not an upstream answer."""


class BadRequest(Exception):
    pass


class TooManyRedirects(Exception):
    pass


class ResolveError(Exception):
    def __init__(self, host: str):
        self.host = host
        super().__init__(host)


# A fetcher serves one request; which proxy a `proxy: true` request egresses through is the
# fetcher's own choice (see engines.ProxyConfig).
Fetcher = Callable[[FetchRequest], Awaitable[FetchResult]]
# A resolver maps a host name to the addresses it resolves to (blocking; run off the loop).
Resolver = Callable[[str], list[str]]


def system_resolver(host: str) -> list[str]:
    return [
        info[4][0] for info in socket.getaddrinfo(host, None, proto=socket.IPPROTO_TCP)
    ]


def is_internal(addr: str) -> bool:
    """Whether an address is one a crawl must never be pointed at: this host, its private
    networks, link-local (the cloud metadata endpoint lives there), and the rest of the
    non-routable space. An address that is not an IP is refused too."""
    try:
        ip = ipaddress.ip_address(addr.split("%", 1)[0])
    except ValueError:
        return True
    return (
        ip.is_private
        or ip.is_loopback
        or ip.is_link_local
        or ip.is_multicast
        or ip.is_reserved
        or ip.is_unspecified
    )


def parse_request(payload) -> FetchRequest:
    """Validate a /fetch payload into a FetchRequest, raising BadRequest on anything malformed."""
    if not isinstance(payload, dict):
        raise BadRequest("body must be a JSON object")
    url = payload.get("url")
    if not isinstance(url, str):
        raise BadRequest("url must be an absolute http(s) URL")
    try:
        parts = urlsplit(url)
    except ValueError:  # e.g. an unclosed IPv6 bracket
        raise BadRequest("url must be an absolute http(s) URL") from None
    if parts.scheme not in ("http", "https") or not parts.netloc:
        raise BadRequest("url must be an absolute http(s) URL")
    mode = payload.get("mode")
    if mode not in MODES:
        raise BadRequest(f"mode must be one of {MODES}")
    method = payload.get("method", "GET")
    if not isinstance(method, str) or method.upper() not in HTTP_METHODS:
        raise BadRequest(f"method must be one of {HTTP_METHODS}")
    method = method.upper()
    if mode == "browser" and method != "GET":
        raise BadRequest("browser mode is a navigation and accepts GET only")
    headers = payload.get("headers", {})
    if not isinstance(headers, dict) or not all(
        isinstance(k, str) and isinstance(v, str) for k, v in headers.items()
    ):
        raise BadRequest("headers must be an object of strings")
    raw_body = payload.get("body", "")
    if not isinstance(raw_body, str):
        raise BadRequest("body must be a base64 string")
    try:
        body = base64.b64decode(raw_body, validate=True)
    except (binascii.Error, ValueError):
        raise BadRequest("body is not valid base64") from None
    proxy = payload.get("proxy", False)
    if not isinstance(proxy, bool):
        raise BadRequest("proxy must be a boolean")
    return FetchRequest(
        url=url, method=method, headers=headers, body=body, mode=mode, proxy=proxy
    )


def _hostname(url: str) -> str | None:
    try:
        return urlsplit(url).hostname
    except ValueError:
        return None


def _header(headers: dict, name: str) -> str | None:
    wanted = name.lower()
    for key, value in headers.items():
        if str(key).lower() == wanted and isinstance(value, str):
            return value
    return None


def redirect_target(base: str, headers: dict) -> str | None:
    """The absolute http(s) URL a redirect points at, or None when there is nothing to follow."""
    raw = _header(headers, "location")
    if raw is None or not raw.strip():
        return None
    try:
        target = urljoin(base, raw.strip())
        parts = urlsplit(target)
    except ValueError:
        raise BadRequest("redirect target must be an absolute http(s) URL") from None
    if parts.scheme not in ("http", "https") or not parts.hostname:
        raise BadRequest("redirect target must be an absolute http(s) URL")
    return target


def request_for_redirect(req: FetchRequest, status: int, target: str) -> FetchRequest:
    method, body = req.method, req.body
    if status in _GET_ON_REDIRECT:
        method, body = "GET", b""
    return replace(req, url=target, method=method, body=body)


async def assert_public(url: str, resolve: Resolver) -> None:
    """Resolve url's host and refuse it when any address is internal.

    Resolution failure is ResolveError (a fetch failure). A denied or unresolvable-to-nothing
    host is BadRequest: the caller asked for a target the crawl must not touch.
    """
    host = _hostname(url)
    if not host:
        raise BadRequest("url must be an absolute http(s) URL")
    try:
        addrs = await asyncio.to_thread(resolve, host)
    except OSError as exc:
        raise ResolveError(host) from exc
    if not addrs or any(is_internal(addr) for addr in addrs):
        raise BadRequest(f"{host} resolves to an internal address")


async def fetch_checked(
    fetcher: Fetcher, req: FetchRequest, resolve: Resolver
) -> FetchResult:
    """Fetch req, following redirects one hop at a time.

    Every hop — the original URL, each Location, and a final URL a fetcher landed on by
    itself — is resolved and held to is_internal before the next request is made. The hop
    count is capped; the target past the cap is not requested.
    """
    current = req
    followed = 0
    while True:
        await assert_public(current.url, resolve)
        result = await fetcher(current)
        landed = result.url if _hostname(result.url or "") else current.url
        if _hostname(landed) != _hostname(current.url):
            await assert_public(landed, resolve)
        if result.status not in REDIRECT_STATUSES:
            return result
        target = redirect_target(landed, result.headers)
        if target is None:
            return result
        if followed >= MAX_REDIRECTS:
            raise TooManyRedirects()
        followed += 1
        current = request_for_redirect(current, result.status, target)


def create_app(
    *,
    resolve: Resolver = system_resolver,
    token: str,
    proxy_available: bool,
    http_fetch: Fetcher,
    browser_fetch: Fetcher,
    max_request_bytes: int,
    max_response_bytes: int,
) -> Starlette:
    expected = f"Bearer {token}".encode()
    fetchers = {"http": http_fetch, "browser": browser_fetch}

    async def health(_: Request) -> JSONResponse:
        return JSONResponse({"status": "ok"})

    async def fetch(request: Request) -> JSONResponse:
        if not hmac.compare_digest(
            request.headers.get("authorization", "").encode(), expected
        ):
            return JSONResponse({"error": "unauthorized"}, status_code=401)
        raw = await request.body()
        if len(raw) > max_request_bytes:
            return JSONResponse({"error": "request body too large"}, status_code=413)
        try:
            req = parse_request(_json(raw))
        except BadRequest as exc:
            return JSONResponse({"error": str(exc)}, status_code=400)
        if req.proxy and not proxy_available:
            return JSONResponse(
                {"error": "proxy requested but none is configured"}, status_code=400
            )
        # Some adapters follow links scraped from the source's own pages, and the fetch runs on
        # the crawl host: a link pointing inside it is refused, the check safehttp makes for
        # the plain client. Checked on every redirect hop, even through the proxy, which would
        # resolve the target remotely.
        try:
            result = await fetch_checked(fetchers[req.mode], req, resolve)
        except BadRequest as exc:
            return JSONResponse({"error": str(exc)}, status_code=400)
        except TooManyRedirects:
            return JSONResponse({"error": "too many redirects"}, status_code=502)
        except (ResolveError, OSError):
            log.exception("name resolution failed")
            return JSONResponse({"error": "fetch failed"}, status_code=502)
        except FetchError:
            log.exception("fetch failed")
            return JSONResponse({"error": "fetch failed"}, status_code=502)
        if len(result.body) > max_response_bytes:
            return JSONResponse(
                {"error": f"upstream body exceeds {max_response_bytes} bytes"},
                status_code=502,
            )
        return JSONResponse(
            {
                "status": result.status,
                "url": result.url,
                "headers": result.headers,
                "body": base64.b64encode(result.body).decode(),
            }
        )

    return Starlette(
        routes=[Route("/health", health), Route("/fetch", fetch, methods=["POST"])]
    )


def _json(raw: bytes):
    try:
        return json.loads(raw)
    except ValueError:
        raise BadRequest("body is not JSON") from None
