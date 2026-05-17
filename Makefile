GOHOSTOS:=$(shell go env GOHOSTOS)
GOPATH:=$(shell go env GOPATH)
VERSION=$(shell git describe --tags --always)

ifeq ($(GOHOSTOS), windows)
	Git_Bash=$(subst \,/,$(subst cmd\,bin\bash.exe,$(dir $(shell where git))))
	INTERNAL_PROTO_FILES=$(shell $(Git_Bash) -c "find internal -name *.proto")
	API_PROTO_FILES=$(shell $(Git_Bash) -c "find api -name *.proto")
else
	INTERNAL_PROTO_FILES=$(shell find internal -name *.proto)
	API_PROTO_FILES=$(shell find api -name *.proto)
endif

.PHONY: run
# run locally
run:
	GOFLAGS='-mod=readonly' kratos run -w ./configs

.PHONY: start
# start docker container locally
start:
	docker compose build && docker compose up -d

.PHONY: stop
# stop docker container locally
stop:
	docker compose down

.PHONY: config
# generate internal proto
config:
	protoc --proto_path=./internal \
	       --proto_path=./third_party \
	       --go_out=paths=source_relative:./internal \
	       $(INTERNAL_PROTO_FILES)

.PHONY: ent
# generate ent
ent:
	go generate ./ent

.PHONY: migrations
# generate migrations
migrations:
	atlas migrate diff init \
		--dir "file://ent/migrate/migrations" \
		--to "ent://ent/schema" \
		--dev-url "docker://postgres/15/test?search_path=public"

.PHONY: api
# generate api proto files
api:
	protoc --proto_path=. \
		   --proto_path=./third_party \
	       --go_out=paths=source_relative:. \
	       --go-grpc_out=paths=source_relative:. \
		   --go-errors_out=paths=source_relative:. \
	       $(API_PROTO_FILES)

.PHONY: build
# build executable file
build:
	mkdir -p bin/ && go build -ldflags "-X main.Version=$(VERSION)" -o ./bin/ ./...

.PHONY: generate
# generate ent & wire
generate:
	go mod tidy
	go get github.com/google/wire/cmd/wire@latest
	GOFLAGS='-mod=readonly' go generate ./...

.PHONY: all
# generate all
all:
	make api;
	make config;
	make generate;
	go mod tidy;

.PHONY: lint
# run linter
lint:
	golangci-lint run ./...

.PHONY: test
# run tests
test:
	go test -v -count=1 ./...

.PHONY: race
# run tests with race
race:
	go test -v -race -count=10 ./...

# show help
help:
	@echo ''
	@echo 'Usage:'
	@echo ' make [target]'
	@echo ''
	@echo 'Targets:'
	@awk '/^[a-zA-Z\-\_0-9]+:/ { \
	helpMessage = match(lastLine, /^# (.*)/); \
		if (helpMessage) { \
			helpCommand = substr($$1, 0, index($$1, ":")); \
			helpMessage = substr(lastLine, RSTART + 2, RLENGTH); \
			printf "\033[36m%-22s\033[0m %s\n", helpCommand,helpMessage; \
		} \
	} \
	{ lastLine = $$0 }' $(MAKEFILE_LIST)

.DEFAULT_GOAL := help
