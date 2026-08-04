# 构建 Next.js 前端产物。
FROM oven/bun:1.3.13 AS web-build

WORKDIR /app/web
ARG NEXT_PUBLIC_DOC_URL=https://docs.canvas.best
ARG NEXT_PUBLIC_REPOSITORY_URL=https://github.com/yypyyd/infinite-canvas
ARG NEXT_PUBLIC_VERSION_URL=https://raw.githubusercontent.com/yypyyd/infinite-canvas/main/VERSION
ARG NEXT_PUBLIC_CREDIT_PURCHASE_URL=https://pay.ldxp.cn/shop/ODLR2IW6
ENV NEXT_PUBLIC_DOC_URL=${NEXT_PUBLIC_DOC_URL}
ENV NEXT_PUBLIC_REPOSITORY_URL=${NEXT_PUBLIC_REPOSITORY_URL}
ENV NEXT_PUBLIC_VERSION_URL=${NEXT_PUBLIC_VERSION_URL}
ENV NEXT_PUBLIC_CREDIT_PURCHASE_URL=${NEXT_PUBLIC_CREDIT_PURCHASE_URL}
COPY web/package.json web/bun.lock ./
RUN --mount=type=cache,target=/root/.bun/install/cache bun install --frozen-lockfile --cache-dir=/root/.bun/install/cache
COPY VERSION /app/VERSION
COPY CHANGELOG.md /app/CHANGELOG.md
COPY web ./
RUN bun run build

# 构建 Go 后端入口。
FROM golang:1.25-alpine AS api-build

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
COPY main.go ./
RUN go build -o /server . \
 && go build -o /batch-worker ./cmd/batch-worker \
 && go build -o /ops-backup ./cmd/ops-backup

# 运行镜像：Next.js 对外监听 3000，Go 只在容器内部监听 8080。
FROM node:22-bookworm-slim

WORKDIR /app
COPY VERSION /app/VERSION
COPY CHANGELOG.md /app/CHANGELOG.md
COPY --from=api-build /server /app/server
COPY --from=api-build /batch-worker /app/batch-worker
COPY --from=api-build /ops-backup /app/ops-backup
COPY --from=web-build /app/web/public /app/web/public
COPY --from=web-build /app/web/.next/standalone /app/web
COPY --from=web-build /app/web/.next/static /app/web/.next/static
ENV NODE_ENV=production
ENV HOSTNAME=0.0.0.0
ENV PORT=3000
ENV PROMPT_DATA_DIR=/app/data/prompts
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates default-mysql-client postgresql-client && rm -rf /var/lib/apt/lists/*
RUN mkdir -p /app/data/prompts

EXPOSE 3000
# 先启动内部 Go API，再由 Next.js 提供页面并代理 /api/*。
CMD ["sh", "-c", "PORT=8080 /app/server & cd /app/web && PORT=3000 node server.js"]
