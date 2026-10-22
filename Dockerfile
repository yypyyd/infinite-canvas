# 使用 Bun 严格安装锁文件，使用与生产一致的 Node.js 构建 Next.js。
FROM oven/bun:1.3.13 AS bun-runtime

FROM node:22-bookworm-slim AS web-build

COPY --from=bun-runtime /usr/local/bin/bun /usr/local/bin/bun

WORKDIR /app/web
ARG NEXT_PUBLIC_DOC_URL=https://docs.canvas.best
ARG NEXT_PUBLIC_REPOSITORY_URL=https://github.com/yypyyd/infinite-canvas
ARG NEXT_PUBLIC_VERSION_URL=https://raw.githubusercontent.com/yypyyd/infinite-canvas/main/VERSION
ENV NEXT_PUBLIC_DOC_URL=${NEXT_PUBLIC_DOC_URL}
ENV NEXT_PUBLIC_REPOSITORY_URL=${NEXT_PUBLIC_REPOSITORY_URL}
ENV NEXT_PUBLIC_VERSION_URL=${NEXT_PUBLIC_VERSION_URL}
COPY web/package.json web/bun.lock ./
RUN --mount=type=cache,target=/root/.bun/install/cache bun install --frozen-lockfile --cache-dir=/root/.bun/install/cache
COPY VERSION /app/VERSION
COPY CHANGELOG.md /app/CHANGELOG.md
COPY web ./
RUN node node_modules/next/dist/bin/next build

# 构建 Go 后端入口。
FROM golang:1.26.6-alpine AS api-build

ENV PATH="/usr/local/go/bin:${PATH}"
WORKDIR /app
COPY go.mod go.sum ./
COPY config ./config
COPY handler ./handler
COPY middleware ./middleware
COPY model ./model
COPY repository ./repository
COPY router ./router
COPY service ./service
COPY cmd ./cmd
COPY gateway ./gateway
COPY main.go ./
RUN go build -o /server . \
 && go build -o /batch-worker ./cmd/batch-worker \
 && go build -o /ops-backup ./cmd/ops-backup \
 && go build -o /gateway-import ./cmd/gateway-import \
 && go build -o /gateway-init-bucket ./gateway/cmd/init-bucket

# 运行镜像：Next.js 对外监听 3000，Go 只在容器内部监听 8080。
FROM node:22-bookworm-slim

WORKDIR /app
COPY VERSION /app/VERSION
COPY CHANGELOG.md /app/CHANGELOG.md
COPY LICENSE NOTICE.md THIRD_PARTY_NOTICES.md /app/
COPY --from=api-build /server /app/server
COPY --from=api-build /batch-worker /app/batch-worker
COPY --from=api-build /ops-backup /app/ops-backup
COPY --from=api-build /gateway-import /app/gateway-import
COPY --from=api-build /gateway-init-bucket /app/gateway-init-bucket
COPY gateway/LICENSE /app/gateway/LICENSE
COPY --from=web-build /app/web/public /app/web/public
COPY --from=web-build /app/web/.next/standalone /app/web
COPY --from=web-build /app/web/.next/static /app/web/.next/static
ENV NODE_ENV=production
ENV HOSTNAME=0.0.0.0
ENV PORT=3000
ENV PROMPT_DATA_DIR=/app/data/prompts
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates default-mysql-client postgresql-client && rm -rf /var/lib/apt/lists/*
RUN mkdir -p /app/data/prompts /app/data/generated

EXPOSE 3000
# 先启动内部 Go API，再由 Next.js 提供页面并代理 /api/*。
# 任一进程退出即让容器退出，交由 restart 策略重启，避免 Go 后端失败后只剩页面在跑。
HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
  CMD node -e "fetch('http://127.0.0.1:8080/api/health').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"
CMD ["bash", "-c", "PORT=8080 /app/server & (cd /app/web && PORT=3000 exec node server.js) & wait -n; status=$?; echo \"process exited with status $status\" >&2; exit $status"]
