FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git make protobuf

COPY . /src
WORKDIR /src

RUN touch .env && CGO_ENABLED=0 go build -o bin/app ./cmd/app/

FROM alpine:3.21

RUN apk add --no-cache ca-certificates

COPY --from=builder /src/bin/app /app/app
COPY --from=builder /src/configs/config.dev.yaml /app/config.yaml

WORKDIR /app

EXPOSE 8000 9000

CMD ["./app", "-conf", "config.yaml"]
