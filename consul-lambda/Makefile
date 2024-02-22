SHELL = /usr/bin/env bash -euo pipefail -c

# ---------- CRT ----------
BIN_NAME = consul-lambda-registrator

ARCH     ?= $(shell A=$$(uname -m); [ $$A = x86_64 ] && A=amd64; echo $$A)
OS       = linux
PLATFORM = $(OS)/$(ARCH)
DIST     = dist/$(PLATFORM)
BIN      = $(DIST)/$(BIN_NAME)

BIN_NAME ?= consul-lambda-registrator
VERSION ?= $(shell ./build-scripts/version.sh)

GIT_COMMIT ?= $(shell git rev-parse --short HEAD)
GIT_DIRTY ?= $(shell test -n "`git status --porcelain`" && echo "+CHANGES" || true)
GIT_DESCRIBE=$(shell git describe --tags --always)
PROJECT = $(shell go list -m)
LD_FLAGS ?= -X "$(PROJECT)/version.GitCommit=$(GIT_COMMIT)$(GIT_DIRTY)"

version:
	@echo $(VERSION)
.PHONY: version

dist:
	mkdir -p $(DIST)

dev: dist
	cd consul-lambda-registrator && GOARCH=$(ARCH) GOOS=$(OS) CGO_ENABLED=0 go build -ldflags "$(LD_FLAGS)" -o $(BIN)
.PHONY: dev

# Docker Stuff.
# TODO: Docker in CircleCI doesn't support buildkit.
#       So we enable build-kit in the individual targets.
#       We can set this here one time, once we're off CircleCI.
# export DOCKER_BUILDKIT=1
BUILD_ARGS = BIN_NAME=consul-lambda-registrator VERSION=$(VERSION) GIT_COMMIT=$(GIT_COMMIT) GIT_DIRTY=$(GIT_DIRTY) ARCH=$(ARCH)
TAG        ?= $(BIN_NAME)/$(TARGET):$(VERSION)
BA_FLAGS   = $(addprefix --build-arg=,$(BUILD_ARGS))
FLAGS      = --target $(TARGET) --platform $(PLATFORM) --tag $(TAG) $(BA_FLAGS)

# Set OS to linux for all docker/* targets.
docker: OS = linux
docker: TARGET = release-default
docker: dev  # TODO
	export DOCKER_BUILDKIT=1; docker build $(FLAGS) ./consul-lambda-registrator
.PHONY: docker
