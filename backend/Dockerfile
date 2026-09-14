# 多阶段构建：编译 → 精简运行镜像（非 root + HEALTHCHECK）
FROM golang:1.23-alpine AS build
ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/api .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget tzdata \
  && adduser -D -u 10001 app \
  && mkdir -p /data/uploads && chown -R app:app /data
ENV TZ=Asia/Shanghai
USER app
WORKDIR /home/app
COPY --from=build /out/api /home/app/api
ENV PORT=8080
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=3s --start-period=10s \
  CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
CMD ["/home/app/api"]
