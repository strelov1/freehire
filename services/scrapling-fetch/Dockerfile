# The fetch service on Scrapling's own image, pinned by digest: the service reaches into
# Scrapling's session API, and a floating tag would change it without a diff here. That image
# already carries Chromium, curl_cffi, starlette and uvicorn, so nothing is installed on top.
FROM ghcr.io/d4vinci/scrapling@sha256:1d876879813759959bc4ba6fbb1ce8247ebbc2b67eee77b140baf7c80af6ae6b

COPY app.py engines.py pool.py main.py /service/
WORKDIR /service

# Relocate the base image's installed browsers out of /root. They remain root-owned and
# read-only to the service; Playwright's per-session profiles and caches live under /tmp.
RUN mv /root/.cache/ms-playwright /opt/ms-playwright \
    && chmod -R a+rX /opt/ms-playwright \
    && groupadd --gid 10001 fetch \
    && useradd --uid 10001 --gid fetch --no-create-home --home-dir /tmp --shell /usr/sbin/nologin fetch
ENV PLAYWRIGHT_BROWSERS_PATH=/opt/ms-playwright \
    HOME=/tmp \
    XDG_CACHE_HOME=/tmp/.cache \
    PYTHONDONTWRITEBYTECODE=1
USER 10001:10001
ENTRYPOINT ["/app/.venv/bin/python", "/service/main.py"]
