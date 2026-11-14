FROM oven/bun:1.3.0 AS build
WORKDIR /src
COPY package.json bun.lock ./
RUN bun install --frozen-lockfile
COPY . .
RUN bun run build

FROM nginx:1.28-alpine
RUN adduser -D -u 10001 app \
    && mkdir -p /tmp/nginx /usr/share/nginx/html \
    && chown -R app:app /tmp/nginx /usr/share/nginx/html
COPY nginx.conf /etc/nginx/nginx.conf
COPY --from=build --chown=app:app /src/dist /usr/share/nginx/html
USER app
EXPOSE 8080
ENTRYPOINT ["nginx"]
CMD ["-g", "daemon off;"]
