import base64

import pytest
from starlette.testclient import TestClient

from app import FetchError, FetchResult, create_app

TOKEN = "t0ken"
AUTH = {"Authorization": f"Bearer {TOKEN}"}


class Recorder:
    """A fake fetcher that records what it was asked and answers a canned result."""

    def __init__(self, result=None, error=None):
        self.calls = []
        self.result = result or FetchResult(
            status=200,
            url="https://ex.com/final",
            headers={"content-type": "text/html"},
            body=b"<html>ok</html>",
        )
        self.error = error

    async def __call__(self, req):
        self.calls.append(req)
        if self.error:
            raise self.error
        return self.result


def public_resolver(host):
    return ["93.184.216.34"]


def client(
    http=None,
    browser=None,
    proxy_available=True,
    max_request=1024,
    max_response=1024,
    resolve=public_resolver,
):
    app = create_app(
        resolve=resolve,
        token=TOKEN,
        proxy_available=proxy_available,
        http_fetch=http or Recorder(),
        browser_fetch=browser or Recorder(),
        max_request_bytes=max_request,
        max_response_bytes=max_response,
    )
    return TestClient(app)


def test_health_needs_no_token():
    r = client().get("/health")
    assert r.status_code == 200 and r.json() == {"status": "ok"}


def test_missing_or_wrong_token_is_401_and_fetches_nothing():
    http = Recorder()
    c = client(http=http)
    assert (
        c.post("/fetch", json={"url": "https://ex.com", "mode": "http"}).status_code
        == 401
    )
    assert (
        c.post(
            "/fetch",
            json={"url": "https://ex.com", "mode": "http"},
            headers={"Authorization": "Bearer nope"},
        ).status_code
        == 401
    )
    assert http.calls == []


@pytest.mark.parametrize(
    "payload",
    [
        {"mode": "http"},  # no url
        {"url": "ftp://ex.com", "mode": "http"},  # not http(s)
        {"url": "http://[::1", "mode": "http"},  # urlsplit raises on it
        {"url": "https://ex.com", "mode": "teleport"},
        {
            "url": "https://ex.com",
            "mode": "browser",
            "method": "POST",
        },  # browser is GET-only
        {
            "url": "https://ex.com",
            "mode": "http",
            "method": "PATCH",
        },  # the http fetcher has no PATCH
        {"url": "https://ex.com", "mode": "http", "body": "%%%not-base64%%%"},
        {"url": "https://ex.com", "mode": "http", "headers": ["not", "a", "map"]},
    ],
)
def test_invalid_requests_are_400(payload):
    http, browser = Recorder(), Recorder()
    r = client(http=http, browser=browser).post("/fetch", json=payload, headers=AUTH)
    assert r.status_code == 400, r.text
    assert http.calls == [] and browser.calls == []


def test_proxy_requested_without_one_configured_is_400():
    http = Recorder()
    r = client(http=http, proxy_available=False).post(
        "/fetch",
        json={"url": "https://ex.com", "mode": "http", "proxy": True},
        headers=AUTH,
    )
    assert r.status_code == 400
    assert http.calls == []


def test_request_body_over_cap_is_413():
    r = client(max_request=64).post(
        "/fetch",
        content=b"{" + b" " * 100 + b"}",
        headers={**AUTH, "content-type": "application/json"},
    )
    assert r.status_code == 413


def test_http_fetch_passes_request_through_and_returns_upstream_as_data():
    http = Recorder(
        result=FetchResult(
            status=403, url="https://ex.com/x", headers={"x": "1"}, body=b"denied"
        )
    )
    body = base64.b64encode(b'{"q":1}').decode()
    r = client(http=http).post(
        "/fetch",
        json={
            "url": "https://ex.com/x",
            "mode": "http",
            "method": "post",
            "headers": {"Accept": "application/json"},
            "body": body,
            "proxy": True,
        },
        headers=AUTH,
    )
    assert r.status_code == 200
    out = r.json()
    assert (
        out["status"] == 403
        and out["url"] == "https://ex.com/x"
        and out["headers"] == {"x": "1"}
    )
    assert base64.b64decode(out["body"]) == b"denied"
    req = http.calls[0]
    assert (
        req.method == "POST"
        and req.body == b'{"q":1}'
        and req.headers == {"Accept": "application/json"}
    )
    assert req.proxy is True


def test_direct_fetch_asks_for_no_proxy():
    http = Recorder()
    client(http=http).post(
        "/fetch", json={"url": "https://ex.com", "mode": "http"}, headers=AUTH
    )
    assert http.calls[0].proxy is False


def test_browser_mode_uses_the_browser_fetcher():
    http, browser = Recorder(), Recorder()
    r = client(http=http, browser=browser).post(
        "/fetch", json={"url": "https://ex.com", "mode": "browser"}, headers=AUTH
    )
    assert r.status_code == 200
    assert len(browser.calls) == 1 and http.calls == []


def test_service_failure_is_502_without_upstream_status():
    http = Recorder(error=FetchError("browser crashed"))
    r = client(http=http).post(
        "/fetch", json={"url": "https://ex.com", "mode": "http"}, headers=AUTH
    )
    assert r.status_code == 502
    assert "status" not in r.json() and "browser crashed" in r.json()["error"]


def test_upstream_body_over_cap_is_502():
    http = Recorder(
        result=FetchResult(
            status=200, url="https://ex.com", headers={}, body=b"x" * 2000
        )
    )
    r = client(http=http, max_response=1000).post(
        "/fetch", json={"url": "https://ex.com", "mode": "http"}, headers=AUTH
    )
    assert r.status_code == 502


@pytest.mark.parametrize(
    "addr",
    [
        "127.0.0.1",
        "10.0.0.5",
        "172.16.3.4",
        "192.168.1.1",
        "169.254.169.254",
        "::1",
        "fd00::1",
        "0.0.0.0",
    ],
)
def test_internal_targets_are_refused(addr):
    # Some adapters follow links scraped from the source's own pages; one pointing inside the
    # crawl host must not be fetched by a browser running there.
    http = Recorder()
    r = client(http=http, resolve=lambda host: [addr]).post(
        "/fetch", json={"url": "http://evil.example/x", "mode": "http"}, headers=AUTH
    )
    assert r.status_code == 400, r.text
    assert http.calls == []


def test_a_host_with_any_internal_address_is_refused():
    http = Recorder()
    r = client(http=http, resolve=lambda host: ["93.184.216.34", "10.0.0.1"]).post(
        "/fetch", json={"url": "https://ex.com", "mode": "http"}, headers=AUTH
    )
    assert r.status_code == 400
    assert http.calls == []


def test_an_unresolvable_host_is_a_fetch_failure_not_a_bad_request():
    def fail(host):
        raise OSError("no such host")

    http = Recorder()
    r = client(http=http, resolve=fail).post(
        "/fetch", json={"url": "https://nowhere.invalid", "mode": "http"}, headers=AUTH
    )
    assert r.status_code == 502
    assert http.calls == []
