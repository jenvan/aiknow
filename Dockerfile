# syntax=docker/dockerfile:1

# ---- 构建阶段 ----
FROM golang:1.25-alpine AS builder

# git 用于 go build 读取版本信息；ca-certificates 供拉取依赖使用
RUN apk add --no-cache git ca-certificates

WORKDIR /src

# 先单独拷贝依赖描述文件，使依赖层在源码变动时仍可复用缓存
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

ARG VERSION=dev
ARG GIT_COMMIT=unknown

# CGO_ENABLED=0 产出静态二进制，便于在无 libc 的镜像中运行
# 静态资源与迁移脚本均通过 go:embed 打包进二进制，运行时无需额外文件
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux \
    go build -trimpath \
        -ldflags "-s -w -X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT}" \
        -o /out/aiknow-server ./cmd/aiknow-server

# ---- 运行阶段 ----
FROM alpine:3.20

# ca-certificates：调用 LLM / Embedding 等 HTTPS 服务所需
# tzdata：容器内使用本地时区
# wget：供 HEALTHCHECK 使用
RUN apk add --no-cache ca-certificates tzdata wget

ENV TZ=Asia/Shanghai

# 以非 root 运行
RUN addgroup -g 10001 -S aiknow \
 && adduser -u 10001 -S aiknow -G aiknow

WORKDIR /app

COPY --from=builder /out/aiknow-server /app/aiknow-server

USER aiknow

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD wget -qO- "http://127.0.0.1:${PORT:-8080}/api/v1/health" >/dev/null 2>&1 || exit 1

# serve 为必需的子命令：不带子命令时程序只打印帮助信息后退出
ENTRYPOINT ["/app/aiknow-server"]
CMD ["serve"]
