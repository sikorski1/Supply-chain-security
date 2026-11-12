FROM ghcr.io/astral-sh/uv:0.13.8 AS uv

FROM python:3.12.12-slim-bookworm
COPY --from=uv /uv /usr/local/bin/uv

WORKDIR /app

ENV UV_COMPILE_BYTECODE=1 \
    UV_LINK_MODE=copy \
    UV_NO_DEV=1 \
    UV_PYTHON_DOWNLOADS=never \
    PATH="/app/.venv/bin:$PATH"

COPY pyproject.toml uv.lock ./
RUN uv sync --frozen --no-dev

COPY main.py consumer.py db.py health.py ./
COPY migrations ./migrations

RUN useradd --uid 10001 --create-home app
USER app

CMD ["python", "main.py"]
