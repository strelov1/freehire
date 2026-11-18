import asyncio

import pytest

from pool import PoolBusy, PoolClosed, SessionPool, UseTimedOut


class FakeSession:
    def __init__(self, key, n):
        self.key, self.n, self.closed = key, n, False

    async def close(self):
        self.closed = True


class Factory:
    def __init__(self):
        self.made = []

    async def __call__(self, key):
        s = FakeSession(key, len(self.made))
        self.made.append(s)
        return s


class Clock:
    def __init__(self):
        self.t = 0.0

    def __call__(self):
        return self.t


def run(coro):
    return asyncio.run(coro)


def test_sequential_uses_on_one_key_reuse_the_session():
    f = Factory()
    pool = SessionPool(f, max_sessions=3, idle_ttl=60)

    async def go():
        a = await pool.run(("ex.com", None), lambda s: _ret(s))
        b = await pool.run(("ex.com", None), lambda s: _ret(s))
        return a, b

    a, b = run(go())
    assert a is b and len(f.made) == 1


def test_different_keys_get_different_sessions():
    f = Factory()
    pool = SessionPool(f, max_sessions=3, idle_ttl=60)

    async def go():
        a = await pool.run(("ex.com", None), lambda s: _ret(s))
        b = await pool.run(("ex.com", "http://proxy"), lambda s: _ret(s))
        return a, b

    a, b = run(go())
    assert a is not b and len(f.made) == 2


def test_concurrent_uses_of_one_key_open_parallel_sessions_under_the_cap():
    f = Factory()
    pool = SessionPool(f, max_sessions=2, idle_ttl=60)
    peak = {"now": 0, "max": 0}

    async def work(s):
        peak["now"] += 1
        peak["max"] = max(peak["max"], peak["now"])
        await asyncio.sleep(0.01)
        peak["now"] -= 1
        return s

    async def go():
        return await asyncio.gather(
            *(pool.run(("ex.com", None), work) for _ in range(5))
        )

    run(go())
    assert len(f.made) == 2  # never past the cap
    assert peak["max"] == 2


def test_cap_reached_evicts_an_idle_session_of_another_key():
    f = Factory()
    pool = SessionPool(f, max_sessions=1, idle_ttl=60)

    async def go():
        await pool.run(("a.com", None), lambda s: _ret(s))
        await pool.run(("b.com", None), lambda s: _ret(s))

    run(go())
    assert len(f.made) == 2
    assert f.made[0].closed and not f.made[1].closed


def test_cap_reached_with_all_busy_waits_for_a_release():
    f = Factory()
    pool = SessionPool(f, max_sessions=1, idle_ttl=60)
    order = []

    async def slow(s):
        order.append("a-start")
        await asyncio.sleep(0.02)
        order.append("a-end")
        return s

    async def fast(s):
        order.append("b")
        return s

    async def go():
        t = asyncio.create_task(pool.run(("a.com", None), slow))
        await asyncio.sleep(0.005)
        await pool.run(("b.com", None), fast)
        await t

    run(go())
    assert order == ["a-start", "a-end", "b"]
    assert len(f.made) == 2


def test_a_failing_use_discards_its_session():
    f = Factory()
    pool = SessionPool(f, max_sessions=2, idle_ttl=60)

    async def boom(s):
        raise RuntimeError("page crashed")

    async def go():
        with pytest.raises(RuntimeError):
            await pool.run(("ex.com", None), boom)
        return await pool.run(("ex.com", None), lambda s: _ret(s))

    s = run(go())
    assert f.made[0].closed and s is f.made[1]


def test_reap_closes_sessions_idle_past_the_ttl():
    f = Factory()
    clock = Clock()
    pool = SessionPool(f, max_sessions=3, idle_ttl=60, clock=clock)

    async def go():
        await pool.run(("a.com", None), lambda s: _ret(s))
        clock.t = 30
        await pool.run(("b.com", None), lambda s: _ret(s))
        clock.t = 75  # a idle 75s, b idle 45s
        await pool.reap()

    run(go())
    assert f.made[0].closed and not f.made[1].closed
    assert pool.size() == 1


def test_a_failing_factory_frees_its_reserved_slot():
    calls = {"n": 0}

    async def flaky(key):
        calls["n"] += 1
        if calls["n"] == 1:
            raise RuntimeError("chrome failed to start")
        return FakeSession(key, calls["n"])

    pool = SessionPool(flaky, max_sessions=1, idle_ttl=60)

    async def go():
        with pytest.raises(RuntimeError):
            await pool.run(("ex.com", None), lambda s: _ret(s))
        return await pool.run(("ex.com", None), lambda s: _ret(s))

    assert run(go()).n == 2


def test_close_all_closes_every_session():
    f = Factory()
    pool = SessionPool(f, max_sessions=3, idle_ttl=60)

    async def go():
        await pool.run(("a.com", None), lambda s: _ret(s))
        await pool.run(("b.com", None), lambda s: _ret(s))
        await pool.close_all()

    run(go())
    assert all(s.closed for s in f.made) and pool.size() == 0


async def _ret(s):
    return s


def test_waiting_past_the_acquire_timeout_gives_up():
    f = Factory()
    pool = SessionPool(f, max_sessions=1, idle_ttl=60, acquire_timeout=0.02)

    async def hold(s):
        await asyncio.sleep(0.2)
        return s

    async def go():
        t = asyncio.create_task(pool.run(("a.com", None), hold))
        await asyncio.sleep(0.005)
        with pytest.raises(PoolBusy):
            await pool.run(("b.com", None), lambda s: _ret(s))
        await t

    run(go())


def test_cancelled_request_does_not_leak_a_slot():

    class SlowClose(FakeSession):
        async def close(self):
            await asyncio.sleep(0.05)
            self.closed = True

    made = []

    async def factory(key):
        s = SlowClose(key, len(made))
        made.append(s)
        return s

    pool = SessionPool(factory, max_sessions=1, idle_ttl=60)

    async def go():
        await pool.run(("a.com", None), lambda s: _ret(s))
        # b evicts a's idle session; cancel b while that slow close is in flight
        t = asyncio.create_task(pool.run(("b.com", None), lambda s: _ret(s)))
        await asyncio.sleep(0.01)
        t.cancel()
        with pytest.raises(asyncio.CancelledError):
            await t
        # the one slot must still be usable
        return await asyncio.wait_for(pool.run(("c.com", None), lambda s: _ret(s)), 1)

    s = run(go())
    assert s.key == ("c.com", None)


def test_close_all_also_closes_a_session_still_opening():

    made = []

    async def slow_factory(key):
        await asyncio.sleep(0.05)
        s = FakeSession(key, len(made))
        made.append(s)
        return s

    pool = SessionPool(slow_factory, max_sessions=2, idle_ttl=60)

    async def go():
        t = asyncio.create_task(pool.run(("a.com", None), lambda s: _ret(s)))
        await asyncio.sleep(0.01)
        await pool.close_all()
        with pytest.raises(PoolClosed):
            await t

    run(go())
    assert made and all(s.closed for s in made) and pool.size() == 0


def test_one_session_serves_up_to_per_session_uses_at_once():
    # A browser takes several tabs; three concurrent uses of one key ride one session.
    f = Factory()
    pool = SessionPool(f, max_sessions=2, idle_ttl=60, per_session=3)

    async def work(s):
        await asyncio.sleep(0.01)
        return s

    async def go():
        return await asyncio.gather(
            *(pool.run(("ex.com", None), work) for _ in range(3))
        )

    got = run(go())
    assert len(f.made) == 1 and all(s is f.made[0] for s in got)


def test_past_per_session_a_second_session_opens():
    f = Factory()
    pool = SessionPool(f, max_sessions=2, idle_ttl=60, per_session=2)

    async def work(s):
        await asyncio.sleep(0.01)
        return s

    async def go():
        return await asyncio.gather(
            *(pool.run(("ex.com", None), work) for _ in range(4))
        )

    run(go())
    assert len(f.made) == 2


def test_a_failed_use_retires_a_shared_session_after_its_other_tabs_finish():
    f = Factory()
    pool = SessionPool(f, max_sessions=2, idle_ttl=60, per_session=2)
    seen_closed_mid_use = []

    async def slow(s):
        await asyncio.sleep(0.03)
        seen_closed_mid_use.append(s.closed)
        return s

    async def boom(s):
        await asyncio.sleep(0.005)
        raise RuntimeError("tab crashed")

    async def go():
        t = asyncio.create_task(pool.run(("ex.com", None), slow))
        await asyncio.sleep(0.001)
        with pytest.raises(RuntimeError):
            await pool.run(("ex.com", None), boom)
        after = await pool.run(
            ("ex.com", None), lambda s: _ret(s)
        )  # must not get the retired one
        await t
        return after

    after = run(go())
    first = f.made[0]
    assert seen_closed_mid_use == [
        False
    ]  # the neighbour tab finished on an open session
    assert first.closed and after is not first


def test_a_session_past_its_max_age_is_retired_and_replaced():
    # A session in constant use never idles out, and Chromium grows over its life, so age alone
    # must retire it.
    f = Factory()
    clock = Clock()
    pool = SessionPool(f, max_sessions=2, idle_ttl=600, max_age=1800, clock=clock)

    async def go():
        a = await pool.run(("ex.com", None), lambda s: _ret(s))
        clock.t = 1801
        b = await pool.run(("ex.com", None), lambda s: _ret(s))
        return a, b

    a, b = run(go())
    assert a is not b and a.closed and not b.closed


def test_a_session_past_its_max_uses_is_retired_and_replaced():
    f = Factory()
    pool = SessionPool(f, max_sessions=2, idle_ttl=600, max_uses=2)

    async def go():
        return [await pool.run(("ex.com", None), lambda s: _ret(s)) for _ in range(3)]

    a, b, c = run(go())
    assert a is b and c is not a and a.closed


def test_a_use_that_hangs_past_the_deadline_frees_its_slot_and_retires_the_session():
    f = Factory()
    pool = SessionPool(f, max_sessions=1, idle_ttl=600, use_timeout=0.02)

    async def hang(s):
        await asyncio.sleep(10)

    async def go():
        with pytest.raises(UseTimedOut):
            await pool.run(("ex.com", None), hang)
        return await asyncio.wait_for(pool.run(("ex.com", None), lambda s: _ret(s)), 1)

    s = run(go())
    assert f.made[0].closed and s is f.made[1]
