# 构建阶段：前端
# 注意：前端产物是静态文件，与 CPU 架构无关。
# multi-arch 构建时如果在 TARGETPLATFORM（例如 linux/arm64）上执行 npm，会触发 QEMU 模拟导致极慢。
# 因此固定在 BUILDPLATFORM 上构建前端。
FROM --platform=$BUILDPLATFORM docker.io/library/node:22.14.0-alpine AS frontend-build

# 设置工作目录
WORKDIR /app/web

# 复制前端依赖文件并安装依赖
COPY ./web/package.json ./web/package-lock.json* ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci --omit=dev --prefer-offline --no-audit --no-fund

# 更新 Browserslist 数据，避免 caniuse-lite 过期警告
RUN true

# 复制前端源代码并构建
COPY ./web/ .
RUN npm run generate

# 将构建结果复制到公共目录
RUN mkdir -p /app/public && cp -r .output/public/* /app/public/

# 构建阶段：后端
# Go 使用 BUILDPLATFORM 交叉编译到 TARGETOS/TARGETARCH，避免 multi-arch 构建时在 QEMU 中运行编译器。
FROM --platform=$BUILDPLATFORM docker.io/library/golang:1.24.1-alpine AS backend-build

# 设置环境变量
ENV CGO_ENABLED=0
ENV GO111MODULE=on
ARG TARGETOS
ARG TARGETARCH

# 设置工作目录
WORKDIR /app

# 复制 Go 模块文件并下载依赖（使用 vendor 模式加速）
COPY ./go.mod ./go.sum ./
RUN go mod download

# 复制项目文件
COPY ./cmd ./cmd
COPY ./internal ./internal
COPY ./pkg ./pkg
COPY ./config ./config

# 编译 Go 应用（使用缓存优化）
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS="${TARGETOS:-linux}" GOARCH="${TARGETARCH:-$(go env GOARCH)}" \
    go build -trimpath -ldflags "-s -w -buildid=" -o /app/noise ./cmd/server/main.go

# 创建必要的目录并设置权限
RUN mkdir -p /app/data /app/public && chmod -R 755 /app/data

# MCP 构建阶段（打包为单文件，避免在最终镜像中保留 node_modules）
# MCP 产物为 JS bundle，与 CPU 架构无关，同样固定在 BUILDPLATFORM 上构建。
FROM --platform=$BUILDPLATFORM docker.io/library/node:20-alpine AS mcp-build
WORKDIR /app/mcp
COPY ./mcp/package.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm install --omit=dev --prefer-offline --no-audit --no-fund
COPY ./mcp/server.js ./server.js
RUN npx --yes esbuild@0.23.0 server.js \
    --bundle \
    --platform=node \
    --format=esm \
    --outfile=server.bundle.mjs

# FFmpeg 构建阶段（尽量静态链接，减少运行时依赖体积）
# 注意：源码编译非常耗时，且 multi-arch 下会被重复构建。
# 默认 final 镜像将改为通过 apk 安装 ffmpeg；如需自编译版本，请使用 --target final-ffmpeg。
FROM docker.io/library/alpine:3.21 AS ffmpeg-build
ARG FFMPEG_VERSION=7.1
RUN set -eux; \
    apk add --no-cache \
      build-base \
      yasm \
      nasm \
      pkgconf \
      x264-dev \
      wget \
      xz \
      tar; \
    mkdir -p /tmp/ffmpeg-src; \
    wget -O /tmp/ffmpeg-src/ffmpeg.tar.xz "https://ffmpeg.org/releases/ffmpeg-${FFMPEG_VERSION}.tar.xz"; \
    tar -xf /tmp/ffmpeg-src/ffmpeg.tar.xz -C /tmp/ffmpeg-src --strip-components=1; \
    cd /tmp/ffmpeg-src; \
    ./configure \
      --prefix=/opt/ffmpeg \
      --bindir=/opt/ffmpeg/bin \
      --disable-debug \
      --disable-doc \
      --disable-ffplay \
      --disable-ffprobe \
      --disable-network \
      --disable-autodetect \
      --disable-shared \
      --enable-static \
      --enable-gpl \
      --enable-libx264 \
      --extra-cflags="-Os"; \
    make -j"$(getconf _NPROCESSORS_ONLN)"; \
    make install; \
    strip /opt/ffmpeg/bin/ffmpeg; \
    /opt/ffmpeg/bin/ffmpeg -version

# 运行时阶段
FROM docker.io/library/alpine:3.21 AS final

# 可选：是否使用 UPX 压缩二进制（1=启用，0=禁用）
ARG USE_UPX=1
# 可选：是否安装 ffmpeg（1=启用，0=禁用）。默认启用以保持原有功能
ARG INSTALL_FFMPEG=1
# 默认使用 apk 版 ffmpeg，避免双架构镜像构建时重复源码编译。
# 如需自编译版本，请显式构建 --target final-ffmpeg。
ARG FFMPEG_MODE=apk
# 镜像版本（用于在运行时展示），构建时可通过 --build-arg VERSION=xxx 传入
ARG VERSION=latest
ENV APP_VERSION=$VERSION
LABEL org.opencontainers.image.version=$VERSION

# 设置工作目录
WORKDIR /app

# 从后端构建阶段复制配置文件和二进制文件
COPY --from=backend-build /app/config /app/config
COPY --from=backend-build /app/config /app/default-config
COPY --from=backend-build /app/noise /app/noise

# 复制docker-compose.yml文件到容器中，用于Docker更新
COPY ./docker-compose.yml /app/docker-compose.yml

# 运行时入口：支持从 /app/config/runtime.env 自动加载环境变量，便于 GUI 容器部署时文件化维护配置
COPY ./docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh
ENTRYPOINT ["/app/docker-entrypoint.sh"]

# 从前端构建阶段复制静态文件
COPY --from=frontend-build /app/public /app/public

# 按需裁剪静态字体：保留仅在 CSS 中引用的 woff2
RUN set -eux; \
    if [ -d /app/public/_nuxt ]; then \
      cd /app/public/_nuxt; \
      ls *.woff2 >/dev/null 2>&1 || exit 0; \
      grep -Eoh '[A-Za-z0-9._-]+\.woff2' -- *.css 2>/dev/null | sort -u > keep.list || true; \
      for f in *.woff2; do \
        grep -qx "$f" keep.list || rm -f "$f"; \
      done; \
      rm -f keep.list; \
    fi

# 额外剔除：删除前端 sourcemap 与许可证文件，减少镜像体积
ARG KEEP_SOURCEMAPS=0
RUN set -eux; \
    if [ "$KEEP_SOURCEMAPS" != "1" ]; then \
      find /app/public -type f -name '*.map' -delete; \
      find /app/public -type f -name '*LICENSE*' -delete; \
      find /app/public -type f -name '.DS_Store' -delete; \
    fi

RUN set -eux; \
    apk add --no-cache ca-certificates; \
    if [ "$INSTALL_FFMPEG" = "1" ]; then \
      apk add --no-cache ffmpeg; \
      ffmpeg -version >/dev/null; \
    fi; \
    rm -rf /usr/share/man /usr/share/doc /usr/share/licenses /usr/share/locale; \
    rm -rf /var/cache/apk/*; \
    mkdir -p /app/data/images; \
    chmod -R 755 /app/data

# 如果需要内置 SQLite 数据库（初始数据）
# COPY ./data/noise.db /app/data/

# 可选：使用 UPX 压缩二进制以减小体积（默认启用，影响极小）
RUN if [ "$USE_UPX" = "1" ]; then \
      apk add --no-cache upx; \
      upx --best --lzma -q /app/noise || true; \
      apk del --no-cache upx || true; \
    fi

# 暴露应用端口
EXPOSE 1314
EXPOSE 1315

# 启动后端与 MCP（MCP 后台运行，Go 服务为主进程）
CMD ["/app/noise"]

# 显式的“带自编译 ffmpeg 的最终镜像”目标。
# 用法：docker build --target final-ffmpeg ...
FROM final AS final-ffmpeg
COPY --from=ffmpeg-build /opt/ffmpeg/bin/ffmpeg /usr/local/bin/ffmpeg
RUN set -eux; \
    apk add --no-cache x264-libs; \
    chmod +x /usr/local/bin/ffmpeg; \
    /usr/local/bin/ffmpeg -version >/dev/null

FROM final AS final-mcp
RUN apk update && \
    apk add --no-cache nodejs && \
    rm -rf /var/cache/apk/*
COPY --from=mcp-build /app/mcp/server.bundle.mjs /app/mcp/server.bundle.mjs
ENV NOTE_HTTP_PORT=1315
EXPOSE 1314
EXPOSE 1315
CMD ["sh","-c","node /app/mcp/server.bundle.mjs & exec /app/noise"]

FROM docker.io/library/node:20-alpine AS mcp-final
WORKDIR /app/mcp
COPY --from=mcp-build /app/mcp/server.bundle.mjs /app/mcp/server.bundle.mjs
ENV NOTE_HTTP_PORT=1315
EXPOSE 1315
CMD ["node","/app/mcp/server.bundle.mjs"]
