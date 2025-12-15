SHELL := bash
.ONESHELL:
.SHELLFLAGS := -eu -o pipefail -c
.DELETE_ON_ERROR:
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules

ENV ?= dev
APP_HOSTNAME ?= point
PORT ?= 4001
STORAGE ?= all
TAG ?= $(ENV)
VERSION ?= HEAD

TAG_CMD=podman tag
PUSH_CMD=podman push

ifeq ($(shell git describe --always > /dev/null 2>&1 ; echo $$?), 0)
	BRANCH=$(shell git rev-parse --abbrev-ref HEAD | tr '/' '-')
	HASH=$(shell git rev-parse --short HEAD)
	VERSION = $(shell printf "%s-%s" "$(BRANCH)" "$(HASH)")
endif
ifeq ($(shell git describe --tags > /dev/null 2>&1 ; echo $$?), 0)
	VERSION = $(shell git describe --tags | tr '/' '-')
endif
ifneq ($(STORAGE),all)
	TAG=$(ENV)-$(STORAGE)
endif

.PHONY: clean images cert build builder push

build:
	ENV=$(ENV) VERSION=$(VERSION) STORAGE=$(STORAGE) PORT=$(PORT) APP_HOSTNAME=$(APP_HOSTNAME) ./image.sh $(APP_HOSTNAME)/app:$(TAG)

push: build
	$(TAG_CMD) "$(APP_HOSTNAME)/app:$(TAG)" "quay.io/go-ap/point:$(TAG)"
	$(PUSH_CMD) "quay.io/go-ap/point:$(TAG)"
ifeq ($(TAG),dev)
	$(TAG_CMD) "$(APP_HOSTNAME)/app:$(TAG)" "quay.io/go-ap/point:latest" || true
	$(PUSH_CMD) "quay.io/go-ap/point:latest" || true
endif
ifneq ($(VERSION),HEAD)
	$(TAG_CMD) "$(APP_HOSTNAME)/app:$(ENV)" "quay.io/go-ap/point:$(VERSION)-$(TAG)" || true
	$(PUSH_CMD) "quay.io/go-ap/point:$(VERSION)-$(TAG)" || true
endif
