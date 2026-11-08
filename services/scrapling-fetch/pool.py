"""A pool of warm browser sessions, keyed by (host, proxy URL).

A Cloudflare clearance belongs to one host and one exit IP, so a session is reused only for the
key it was opened for: the challenge is solved once and every later page of the crawl rides on
it. A session serves up to `per_session` uses at once — one browser, several tabs, far cheaper
than a browser each — and several sessions may share a key when one host needs more than that.
A global cap bounds memory (a browser session peaks at ~1.7 GB); past it, an idle session of
another key is closed to make room, and if none is idle the caller waits — but only for `acquire_timeout`, because a
caller that has given up and retried would otherwise leave its abandoned request queued, and
then spend a browser on a page nobody reads.

The factory is injected, so the pool's tests need no browser.
"""

import asyncio
import time
from collections.abc import Awaitable, Callable, Hashable
from dataclasses import dataclass
from typing import Any


class PoolBusy(Exception):
    """No session came free within the acquire timeout."""


class PoolClosed(Exception):
    """The pool is shutting down."""


class UseTimedOut(Exception):
    """A use ran past the pool's hard per-use deadline; its session is retired."""


@dataclass
class _Entry:
    key: Hashable
    session: Any
    in_use: int
    last_used: float
    created_at: float = 0.0
    uses: int = 0
    # retired: a use of this session failed. It takes no new uses and is closed when its
    # last in-flight one (a neighbouring tab) finishes.
    retired: bool = False


class SessionPool:
    def __init__(
        self,
        factory: Callable[[Hashable], Awaitable[Any]],
        *,
        max_sessions: int,
        idle_ttl: float,
        acquire_timeout: float | None = None,
        per_session: int = 1,
        max_age: float | None = None,
        max_uses: int | None = None,
        use_timeout: float | None = None,
        clock: Callable[[], float] = time.monotonic,
    ):
        if max_sessions < 1 or per_session < 1:
            raise ValueError("max_sessions and per_session must be at least 1")
        self._per = per_session
        # A session in constant use never idles out while Chromium grows over its life, so
        # age and use count retire it too; a use past use_timeout is cut off and its session
        # retired, so one hung tab cannot hold a slot for ever.
        self._max_age = max_age
        self._max_uses = max_uses
        self._use_timeout = use_timeout
        self._factory = factory
        self._max = max_sessions
        self._ttl = idle_ttl
        self._acquire_timeout = acquire_timeout
        self._clock = clock
        self._entries: list[_Entry] = []
        self._opening = 0  # slots reserved by a session still being opened
        self._closed = False
        self._cond = asyncio.Condition()

    def size(self) -> int:
        return len(self._entries)

    async def run(self, key: Hashable, fn: Callable[[Any], Awaitable[Any]]) -> Any:
        """Run fn on a session for key. A session whose use raised is closed, never reused."""
        entry = await self._acquire(key)
        try:
            if self._use_timeout is None:
                result = await fn(entry.session)
            else:
                result = await asyncio.wait_for(fn(entry.session), self._use_timeout)
        except TimeoutError:
            await self._discard(entry)
            raise UseTimedOut(f"a use ran past {self._use_timeout}s") from None
        except BaseException:
            await self._discard(entry)
            raise
        await self._release(entry)
        return result

    async def _acquire(self, key: Hashable) -> _Entry:
        deadline = None if self._acquire_timeout is None else self._clock() + self._acquire_timeout
        evicted = None
        expired: list[_Entry] = []
        async with self._cond:
            while True:
                if self._closed:
                    raise PoolClosed("the pool is closed")
                expired.extend(self._retire_expired())
                for e in self._entries:
                    if e.key == key and not e.retired and e.in_use < self._per:
                        e.in_use += 1
                        e.uses += 1
                        break
                else:
                    e = None
                if e is not None:
                    break
                if len(self._entries) + self._opening < self._max:
                    break
                evicted = self._idle_lru()
                if evicted is not None:
                    self._entries.remove(evicted)
                    break
                await self._wait(deadline)
            if e is None:
                self._opening += 1  # from here on the slot is ours until the session is registered
        if e is not None:
            for x in expired:
                await _close_quietly(x.session)
            return e
        try:
            # Inside the try: a cancellation during these closes must release the reserved slot.
            for x in expired:
                await _close_quietly(x.session)
            if evicted is not None:
                await _close_quietly(evicted.session)  # outside the lock: closing a browser is slow
            session = await self._factory(key)
        except BaseException:
            async with self._cond:
                self._opening -= 1
                self._cond.notify_all()
            raise
        async with self._cond:
            self._opening -= 1
            if self._closed:
                self._cond.notify_all()
                closed = True
            else:
                now = self._clock()
                entry = _Entry(key=key, session=session, in_use=1, last_used=now, created_at=now, uses=1)
                self._entries.append(entry)
                closed = False
        if closed:
            await _close_quietly(session)
            raise PoolClosed("the pool closed while the session was opening")
        return entry

    async def _wait(self, deadline: float | None) -> None:
        if deadline is None:
            await self._cond.wait()
            return
        remaining = deadline - self._clock()
        if remaining <= 0:
            raise PoolBusy(f"no browser session came free within {self._acquire_timeout}s")
        try:
            await asyncio.wait_for(self._cond.wait(), remaining)
        except TimeoutError:
            raise PoolBusy(f"no browser session came free within {self._acquire_timeout}s") from None

    def _retire_expired(self) -> list[_Entry]:
        """Retire every session past its age or use limit; return the idle ones, removed, for
        the caller to close outside the lock. A busy one closes when its last use releases."""
        now = self._clock()
        closing = []
        for e in list(self._entries):
            if e.retired:
                continue
            old = self._max_age is not None and now - e.created_at > self._max_age
            worn = self._max_uses is not None and e.uses >= self._max_uses
            if old or worn:
                e.retired = True
                if e.in_use == 0:
                    self._entries.remove(e)
                    closing.append(e)
        return closing

    def _idle_lru(self):
        idle = [e for e in self._entries if e.in_use == 0]
        return min(idle, key=lambda e: e.last_used) if idle else None

    async def _release(self, entry: _Entry) -> None:
        async with self._cond:
            entry.in_use -= 1
            entry.last_used = self._clock()
            close = entry.retired and entry.in_use == 0
            if close and entry in self._entries:
                self._entries.remove(entry)
            self._cond.notify_all()
        if close:
            await _close_quietly(entry.session)

    async def _discard(self, entry: _Entry) -> None:
        """Retire a session whose use failed: no new use gets it, and it closes once its other
        in-flight uses finish — closing it now would kill the neighbouring tabs."""
        async with self._cond:
            entry.retired = True
        await self._release(entry)

    async def reap(self) -> None:
        """Close every idle session unused for longer than the idle TTL."""
        now = self._clock()
        async with self._cond:
            stale = [e for e in self._entries if e.in_use == 0 and now - e.last_used > self._ttl]
            for e in stale:
                self._entries.remove(e)
            if stale:
                self._cond.notify_all()
        for e in stale:
            await _close_quietly(e.session)

    async def close_all(self) -> None:
        """Close every session and refuse new work; a session still opening closes on arrival."""
        async with self._cond:
            self._closed = True
            entries, self._entries = self._entries, []
            self._cond.notify_all()
        for e in entries:
            await _close_quietly(e.session)


async def _close_quietly(session: Any) -> None:
    """Close a session, finishing the close even if the caller is cancelled meanwhile: a browser
    left half-closed is a leaked process."""
    task = asyncio.ensure_future(session.close())
    try:
        await asyncio.shield(task)
    except asyncio.CancelledError:
        try:
            await task
        except Exception:
            pass
        raise
    except Exception:
        pass  # a session that cannot close cleanly is gone either way
