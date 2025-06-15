# 使用官方 Go 镜像作为构建阶段
FROM golang:1.23

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

# 设置默认命令
CMD ["./ast-callgraph"]