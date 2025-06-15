# 使用官方 Go 镜像作为构建阶段
FROM golang:1.23-alpine AS builder

# 安装 git
RUN apk add --no-cache git

# 设置工作目录
WORKDIR /app

# 克隆指定的 Git 仓库
RUN git clone git@github.com:Silhouette-sophist/ast-callgraph.git .

# 下载依赖
RUN go mod tidy

# 构建应用程序
RUN CGO_ENABLED=0 GOOS=linux go build -o ast-callgraph .

# 使用更小的镜像来运行应用程序
FROM alpine:latest

# 复制构建好的二进制文件
COPY --from=builder /app/ast-callgraph .

# 设置默认命令
CMD ["./ast-callgraph"]