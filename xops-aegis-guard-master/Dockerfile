FROM harbor.zfit888.com/ywcw-base/golang:1.21.1-alpine AS build

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN go build -o run -ldflags="-w -s" ./cmd

FROM harbor.zfit888.com/ywcw-base/alpine:latest
RUN ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
      echo "Asia/Shanghai" > /etc/timezone

WORKDIR /app
COPY --from=build /app/run .

CMD ["./run", "-f", "config/config.yml"]
