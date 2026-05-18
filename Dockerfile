FROM golang:latest AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o bin/app ./cmd/app/

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /src/bin/app /app/app
COPY --from=builder /src/configs/ /app/configs/
WORKDIR /app
EXPOSE 8000 9000
CMD ["./app", "-conf", "configs/config.local.yaml"]
