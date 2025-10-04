COMMIT = $(shell git log --pretty=format:'%h' -n 1)
VERSION=$(shell git describe --abbrev=0 --exact-match || echo development)
GOBUILD_OPTS = -ldflags="-s -w -X main.Version=${VERSION} -X main.CommitHash=${COMMIT}"

lint:
	golangci-lint run -v

modsync:
	go mod tidy && go mod vendor

# Use following target to run directly dns verifier and test functionalities
# You can run it like `make run ARGS="version -h"`
run:
	go run -mod=vendor *.go ${ARGS}

install:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go install -v -a -mod=vendor ${GOBUILD_OPTS}

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -mod=vendor ${GOBUILD_OPTS} -o ./bin/dns-verifier

darwinbuild:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -a -mod=vendor ${GOBUILD_OPTS} -o ./bin/dns-verifier-darwin

build-all:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -a -mod=vendor ${GOBUILD_OPTS} -o ./bin/dns-verifier-darwin && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -mod=vendor ${GOBUILD_OPTS} -o ./bin/dns-verifier

fmt:
	go fmt ./...

test:
	go test -mod=vendor `go list ./... ` -race

help:
	@echo "Please use 'make <target>' where <target> is one of the following:"
	@echo "  run             to run the app without building."
	@echo "  build-all       to build the app for both Linux and MacOSX."
	@echo "  build           to build the app for Linux."
	@echo "  darwinbuild     to build the app for MacOSX."
	@echo "  lint            to perform linting."
	@echo "  fmt             to perform formatting."
	@echo "  modsync         to perform mod tidy and vendor."
	@echo "  test            to run application tests."

.PHONY: help lint run build darwinbuild fmt
