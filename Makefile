SHELL := bash
.ONESHELL:
.SHELLFLAGS := -eu -o pipefail -c
.DELETE_ON_ERROR:
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules

LDFLAGS ?= -X main.version=$(VERSION)
BUILDFLAGS ?= -a -ldflags '$(LDFLAGS)'
TEST_FLAGS ?= -count=1

GO ?= go
ENV ?= dev
STORAGE ?=
APPSOURCES := $(wildcard ./*.go)
PROJECT_NAME := point

TAGS := $(ENV)
ifneq ($(STORAGE), )
	TAGS +=  storage_$(STORAGE)
endif

export CGO_ENABLED=0

ifneq ($(ENV), dev)
	LDFLAGS += -s -w -extldflags "-static"
	BUILDFLAGS += -trimpath
endif

ifeq ($(VERSION), )
	ifeq ($(shell git describe --always > /dev/null 2>&1 ; echo $$?), 0)
		BRANCH=$(shell git rev-parse --abbrev-ref HEAD | tr '/' '-')
		HASH=$(shell git rev-parse --short HEAD)
		VERSION ?= $(shell printf "%s-%s" "$(BRANCH)" "$(HASH)")
	endif
	ifeq ($(shell git describe --tags > /dev/null 2>&1 ; echo $$?), 0)
		VERSION ?= $(shell git describe --tags | tr '/' '-')
	endif
endif

BUILD := $(GO) build $(BUILDFLAGS)
TEST := $(GO) test $(BUILDFLAGS)

.PHONY: all run clean test coverage download

all: point

download:
	$(GO) mod tidy

point: bin/point
bin/point: go.mod cmd/point/main.go $(APPSOURCES)
	$(BUILD) -tags "$(TAGS)" -o $@ ./cmd/point

run: point
	@./bin/point

clean:
	-$(RM) bin/*

test: TEST_TARGET := .
test: download
	$(TEST) $(TEST_FLAGS) -tags "$(TAGS)" $(TEST_TARGET)

coverage: TEST_TARGET := .
coverage: TEST_FLAGS += -covermode=count -coverprofile $(PROJECT_NAME).coverprofile
coverage: test
