SHELL := bash
.ONESHELL:
.SHELLFLAGS := -eu -o pipefail -c
.DELETE_ON_ERROR:
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules

PROJECT_NAME := point
APP_HOSTNAME ?= $(PROJECT_NAME)
ENV ?= dev
STORAGE ?= all
VERSION ?= HEAD

LDFLAGS ?= -X main.version=$(VERSION)
BUILDFLAGS ?= -a -ldflags '$(LDFLAGS)' -tags "$(TAGS)"
TEST_FLAGS ?= -count=1 -tags "$(TAGS)"

UPX = upx
GO ?= go
APPSOURCES := $(wildcard ./*.go cmd/point/*.go)

TAGS := $(ENV)
ifneq ($(STORAGE), )
	TAGS +=  storage_$(STORAGE)
endif

export CGO_ENABLED=0
export GOEXPERIMENT=greenteagc

ifeq ($(shell git describe --always > /dev/null 2>&1 ; echo $$?), 0)
	BRANCH=$(shell git rev-parse --abbrev-ref HEAD | tr '/' '-')
	HASH=$(shell git rev-parse --short HEAD)
	VERSION ?= $(shell printf "%s-%s" "$(BRANCH)" "$(HASH)")
endif
ifeq ($(shell git describe --tags > /dev/null 2>&1 ; echo $$?), 0)
	VERSION ?= $(shell git describe --tags | tr '/' '-')
endif

ifneq ($(ENV), dev)
	LDFLAGS += -s -w -extldflags "-static"
	BUILDFLAGS += -trimpath
endif

BUILD := $(GO) build $(BUILDFLAGS)
TEST := $(GO) test $(BUILDFLAGS)

.PHONY: all point cert clean test coverage download help

.DEFAULT_GOAL := help

help: ## Help target that shows this message.
	@sed -rn 's/^([^:]+):.*[ ]##[ ](.+)/\1:\2/p' $(MAKEFILE_LIST) | column -ts: -l2

all: point

download: go.sum ## Downloads dependencies and tidies the go.mod file.

go.sum: go.mod
	$(GO) mod download all
	$(GO) mod tidy

point: bin/point ## Builds the main WebFinger service binary.
bin/point: go.mod go.sum $(APPSOURCES)
	$(BUILD) -o $@ ./cmd/point
ifneq ($(ENV),dev)
	$(UPX) -q --mono --no-progress --best $@ || true
endif

clean: ## Cleanup the build workspace.
	-$(RM) bin/*
	$(MAKE) -C images $@

test: TEST_TARGET := .
test: go.sum
	$(TEST) $(TEST_FLAGS) $(TEST_TARGET)

coverage: TEST_TARGET := .
coverage: TEST_FLAGS += -covermode=count -coverprofile $(PROJECT_NAME).coverprofile
coverage: test

cert: bin/$(APP_HOSTNAME).pem ## Create a certificate.
bin/$(APP_HOSTNAME).pem: bin/$(APP_HOSTNAME).key bin/$(APP_HOSTNAME).crt
bin/$(APP_HOSTNAME).key bin/$(APP_HOSTNAME).crt:
	./images/gen-certs.sh ./bin/$(APP_HOSTNAME)
