# 项目自带镜像：保留 go 工具链，方便在容器内直接修改并重新编译。
FROM golang:1.21-alpine

ENV GOTOOLCHAIN=local
ENV CGO_ENABLED=0

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build ./...

CMD ["/bin/sh"]
