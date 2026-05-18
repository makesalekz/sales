FROM golang:latest AS builder

RUN apk add --no-cache git make protobuf || apt-get update && apt-get install -y git make protobuf-compiler || true

COPY . /src
WORKDIR /src

RUN touch .env && CGO_ENABLED=0 go build -mod=vendor -o bin/app ./cmd/app/

FROM alpine:3.21

RUN apk add --no-cache ca-certificates

COPY --from=builder /src/bin/app /app/app
COPY --from=builder /src/configs/ /app/configs/

WORKDIR /app

EXPOSE 8000 9000

CMD ["./app", "-conf", "configs/config.dev.yaml"]
