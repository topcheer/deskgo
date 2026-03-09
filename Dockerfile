# 多阶段构建 - 构建阶段
FROM golang:1.24-alpine AS builder

WORKDIR /app

# 安装构建依赖
RUN apk add --no-cache bash git gcc musl-dev

# 复制go mod文件
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 编译中继服务器和多架构下载包
RUN mkdir -p /out \
    && bash ./build.sh \
    && cp bin/relay-server /out/relay-server \
    && cp -R downloads /out/downloads

# 最终镜像
FROM alpine:latest

WORKDIR /app

# 安装运行时依赖
RUN apk --no-cache add ca-certificates tzdata curl

# 复制编译好的二进制文件
COPY --from=builder /out/relay-server .

# 复制web文件
COPY web ./web

# 复制客户端下载包
COPY --from=builder /out/downloads ./downloads

# 创建日志目录
RUN mkdir -p logs

# 暴露端口
EXPOSE 8082

# 健康检查
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD curl -f http://localhost:8082/api/health || exit 1

# 设置环境变量
ENV RELAY_HOST=0.0.0.0
ENV RELAY_PORT=8082

# 运行服务
CMD ["./relay-server"]
