from engines import forwardable_headers


def test_identity_and_transport_headers_are_dropped():
    # The caller's User-Agent would undo the fetcher's browser impersonation, and the
    # transport-level ones are the fetcher's to set.
    got = forwardable_headers({
        "User-Agent": "freehire/0.1",
        "Host": "ex.com",
        "Content-Length": "12",
        "Accept-Encoding": "gzip",
        "Connection": "keep-alive",
        "Accept": "application/json",
        "Content-Type": "application/json",
        "X-Custom": "1",
    })
    assert got == {"Accept": "application/json", "Content-Type": "application/json", "X-Custom": "1"}


def test_result_drops_framing_headers_of_the_already_decoded_body():
    # The fetcher hands back the body decompressed and whole, so the upstream's framing
    # headers describe bytes the caller never receives.
    from engines import _result

    class Page:
        status = 200
        url = "https://ex.com/final"
        body = b"hello"
        headers = {"Content-Encoding": "gzip", "Content-Length": "999", "Transfer-Encoding": "chunked", "Content-Type": "text/html"}

    r = _result(Page())
    assert r.headers == {"Content-Type": "text/html"} and r.body == b"hello" and r.url == "https://ex.com/final"


def test_listen_address_defaults_its_host_to_loopback():
    from main import listen_address

    assert listen_address("") == ("127.0.0.1", 8098)
    assert listen_address(":9000") == ("127.0.0.1", 9000)
    assert listen_address("0.0.0.0:8098") == ("0.0.0.0", 8098)


import asyncio

from app import FetchRequest
from engines import ProxyConfig, browser_fetcher, http_fetcher, stealth_session_factory
from pool import SessionPool


def test_proxy_config_from_env_prefers_the_rotating_gateway_and_sticky_pool():
    cfg = ProxyConfig.from_env({
        "SOURCES_PROXY_URL": "http://u:p@1.2.3.4:5",
        "SCRAPLING_PROXY_ROTATING": "http://u-rotate:p@gw:80",
        "SCRAPLING_PROXY_STICKY": "http://u-{n}:p@gw:80",
        "SCRAPLING_PROXY_STICKY_COUNT": "3",
    })
    assert cfg.available
    assert cfg.rotating == "http://u-rotate:p@gw:80"
    assert [cfg.next_sticky() for _ in range(4)] == ["http://u-1:p@gw:80", "http://u-2:p@gw:80", "http://u-3:p@gw:80", "http://u-1:p@gw:80"]


def test_proxy_config_falls_back_to_the_single_proxy():
    cfg = ProxyConfig.from_env({"SOURCES_PROXY_URL": "http://u:p@1.2.3.4:5"})
    assert cfg.available and cfg.rotating == "http://u:p@1.2.3.4:5" and cfg.next_sticky() == "http://u:p@1.2.3.4:5"
    assert not ProxyConfig.from_env({}).available


def test_proxy_config_refuses_a_sticky_template_without_a_count():
    import pytest

    with pytest.raises(ValueError):
        ProxyConfig.from_env({"SCRAPLING_PROXY_STICKY": "http://u-{n}:p@gw:80"})


def test_http_fetch_uses_the_rotating_proxy_only_when_asked(monkeypatch):
    seen = []

    async def fake_send(url, **kw):
        seen.append(kw.get("proxy"))

        class P:
            status, url, body, headers = 200, "https://ex.com", b"ok", {}

        return P()

    fetch = http_fetcher(ProxyConfig(rotating="http://rot", sticky=[]), send=lambda method: fake_send)
    asyncio.run(fetch(FetchRequest(url="https://ex.com", proxy=True)))
    asyncio.run(fetch(FetchRequest(url="https://ex.com", proxy=False)))
    assert seen == ["http://rot", None]


class FakeBrowser:
    def __init__(self, proxy, statuses):
        self.proxy, self.statuses, self.closed = proxy, statuses, False

    async def fetch(self, url):
        status = self.statuses.pop(0)

        class P:
            pass

        p = P()
        p.status, p.url, p.body, p.headers = status, url, b"x", {}
        return p

    async def close(self):
        self.closed = True


def test_a_refused_browser_session_is_retired_so_the_next_gets_another_ip():
    opened = []
    statuses = [[429], [200]]

    async def open_session(key, proxy, tabs):
        b = FakeBrowser(proxy, statuses.pop(0))
        opened.append(b)
        return b

    cfg = ProxyConfig(rotating=None, sticky=["http://ip1", "http://ip2"])
    pool = SessionPool(stealth_session_factory(cfg, open_session), max_sessions=2, idle_ttl=60)
    fetch = browser_fetcher(pool)

    async def go():
        first = await fetch(FetchRequest(url="https://ex.com/a", mode="browser", proxy=True))
        second = await fetch(FetchRequest(url="https://ex.com/b", mode="browser", proxy=True))
        return first, second

    first, second = asyncio.run(go())
    assert first.status == 429 and second.status == 200  # the refusal is still returned as data
    assert [b.proxy for b in opened] == ["http://ip1", "http://ip2"]
    assert opened[0].closed and not opened[1].closed


def test_a_405_retires_the_browser_session_too():
    # InfoJobs (Adevinta's bot wall) turns a flagged session away with 405, not 403: measured
    # 2026-10-30, after ~45 offer pages one session answered 405 to 942 of the next ~950 until
    # it retired by age, and the next session's exit was served again.
    opened = []
    statuses = [[405], [200]]

    async def open_session(key, proxy, tabs):
        b = FakeBrowser(proxy, statuses.pop(0))
        opened.append(b)
        return b

    cfg = ProxyConfig(rotating=None, sticky=["http://ip1", "http://ip2"])
    pool = SessionPool(stealth_session_factory(cfg, open_session), max_sessions=2, idle_ttl=60)
    fetch = browser_fetcher(pool)

    async def go():
        first = await fetch(FetchRequest(url="https://ex.com/a", mode="browser", proxy=True))
        second = await fetch(FetchRequest(url="https://ex.com/b", mode="browser", proxy=True))
        return first, second

    first, second = asyncio.run(go())
    assert first.status == 405 and second.status == 200
    assert [b.proxy for b in opened] == ["http://ip1", "http://ip2"]
    assert opened[0].closed and not opened[1].closed


def test_a_405_through_the_rotating_proxy_is_not_retried_here():
    # Over http the 405 callers (adzuna-liveness, epreselec) already re-send it themselves with
    # their own budgets; retrying here as well would multiply every one of those attempts.
    calls = []

    async def send_fn(url, **kw):
        calls.append(1)

        class P:
            status, url, body, headers = 405, "https://ex.com", b"pardon", {}

        return P()

    fetch = http_fetcher(ProxyConfig(rotating="http://rot", sticky=[]), send=lambda method: send_fn)
    r = asyncio.run(fetch(FetchRequest(url="https://ex.com", proxy=True)))
    assert r.status == 405 and len(calls) == 1


def test_a_direct_browser_session_gets_no_proxy():
    got = []

    async def open_session(key, proxy, tabs):
        got.append(proxy)
        return FakeBrowser(proxy, [200])

    pool = SessionPool(stealth_session_factory(ProxyConfig(rotating=None, sticky=["http://ip1"]), open_session), max_sessions=1, idle_ttl=60)
    asyncio.run(browser_fetcher(pool)(FetchRequest(url="https://ex.com", mode="browser", proxy=False)))
    assert got == [None]


def test_a_refusal_through_the_rotating_proxy_is_retried_on_a_new_ip():
    # Each request through the rotating gateway leaves from another exit IP, so a 403/429 is
    # usually one flagged address, not the source: a retry lands somewhere else.
    statuses = [403, 429, 200]
    calls = []

    async def send_fn(url, **kw):
        calls.append(kw.get("proxy"))

        class P:
            pass

        p = P()
        p.status, p.url, p.body, p.headers = statuses.pop(0), url, b"x", {}
        return p

    fetch = http_fetcher(ProxyConfig(rotating="http://rot", sticky=[]), send=lambda method: send_fn)
    r = asyncio.run(fetch(FetchRequest(url="https://ex.com", proxy=True)))
    assert r.status == 200 and len(calls) == 3


def test_refusal_retries_are_bounded_and_the_last_answer_returned():
    calls = []

    async def send_fn(url, **kw):
        calls.append(1)

        class P:
            status, url, body, headers = 429, "https://ex.com", b"slow down", {}

        return P()

    fetch = http_fetcher(ProxyConfig(rotating="http://rot", sticky=[]), send=lambda method: send_fn)
    r = asyncio.run(fetch(FetchRequest(url="https://ex.com", proxy=True)))
    assert r.status == 429 and len(calls) == 3


def test_a_direct_refusal_is_not_retried():
    # Without the proxy the next attempt leaves from the same IP: retrying buys nothing.
    calls = []

    async def send_fn(url, **kw):
        calls.append(1)

        class P:
            status, url, body, headers = 403, "https://ex.com", b"no", {}

        return P()

    fetch = http_fetcher(ProxyConfig(rotating="http://rot", sticky=[]), send=lambda method: send_fn)
    r = asyncio.run(fetch(FetchRequest(url="https://ex.com", proxy=False)))
    assert r.status == 403 and len(calls) == 1


def test_a_transport_error_through_the_rotating_proxy_is_retried_on_a_new_ip():
    # A slow or dead exit IP times out; the next attempt leaves from another one.
    outcomes = [TimeoutError("curl: (28) Operation timed out"), "ok"]
    calls = []

    async def send_fn(url, **kw):
        calls.append(1)
        o = outcomes.pop(0)
        if isinstance(o, Exception):
            raise o

        class P:
            status, url, body, headers = 200, "https://ex.com", b"x", {}

        return P()

    fetch = http_fetcher(ProxyConfig(rotating="http://rot", sticky=[]), send=lambda method: send_fn)
    r = asyncio.run(fetch(FetchRequest(url="https://ex.com", proxy=True)))
    assert r.status == 200 and len(calls) == 2


def test_a_transport_error_on_every_attempt_is_a_fetch_error():
    import pytest

    from app import FetchError

    async def send_fn(url, **kw):
        raise TimeoutError("timed out")

    fetch = http_fetcher(ProxyConfig(rotating="http://rot", sticky=[]), send=lambda method: send_fn)
    with pytest.raises(FetchError):
        asyncio.run(fetch(FetchRequest(url="https://ex.com", proxy=True)))
