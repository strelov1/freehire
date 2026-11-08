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
import socket
from collections.abc import Awaitable, Callable
from dataclasses import dataclass, field
from urllib.parse import urlsplit

from starlette.applications import Starlette
from starlette.requests import Request
from starlette.responses import JSONResponse
from starlette.routing import Route

MODES = ("http", "browser")
# The methods Scrapling's HTTP fetcher implements.
HTTP_METHODS = ("GET", "POST", "PUT", "DELETE")


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
    non-routable space."""
    ip = ipaddress.ip_address(addr.split("%", 1)[0])
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
        except BadRequest as e:
            return JSONResponse({"error": str(e)}, status_code=400)
        if req.proxy and not proxy_available:
            return JSONResponse(
                {"error": "proxy requested but none is configured"}, status_code=400
            )
        # Some adapters follow links scraped from the source's own pages, and the fetch runs on
        # the crawl host: a link pointing inside it is refused, the check safehttp makes for
        # the plain client. Checked even through the proxy, which would resolve it remotely.
        host = urlsplit(req.url).hostname
        try:
            addrs = await asyncio.to_thread(resolve, host)
        except OSError as e:
            return JSONResponse({"error": f"resolve {host}: {e}"}, status_code=502)
        if not addrs or any(is_internal(a) for a in addrs):
            return JSONResponse(
                {"error": f"{host} resolves to an internal address"}, status_code=400
            )
        try:
            result = await fetchers[req.mode](req)
        except FetchError as e:
            return JSONResponse({"error": str(e)}, status_code=502)
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
