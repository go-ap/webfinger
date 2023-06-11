SHELL := bash
.ONESHELL:
.SHELLFLAGS := -eu -o pipefail -c
.DELETE_ON_ERROR:
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules

ENV ?= prod
APP_HOSTNAME ?= point.local
PORT ?= 4001

BUILD_CMD=buildah bud
RUN_CMD=podman run
TAG_CMD=podman tag
PUSH_CMD=podman push
COMPOSE_CMD=podman-compose

.PHONY: clean images cert build builder push

point.pem:
	./gen-certs.sh point

cert: point.pem

clean:
	@-$(RM) point.{key,crt,pem}

builder:
	./build.sh .. point/builder

build:
	ENV=$(ENV) VERSION=$(VERSION) PORT=$(PORT) APP_HOSTNAME=$(APP_HOSTNAME) ./image.sh point/app:$(ENV)

push: build
	$(TAG_CMD) point/app:$(ENV) quay.io/go-ap/point:$(ENV)
	$(PUSH_CMD) quay.io/go-ap/point:$(ENV)
ifeq ($(ENV), dev)
	$(TAG_CMD) point/app:$(ENV) quay.io/go-ap/point:latest
	$(PUSH_CMD) quay.io/go-ap/point:latest || true
endif
ifneq ($(VERSION), )
	$(TAG_CMD) point/app:$(ENV) quay.io/go-ap/point:$(VERSION)-$(ENV) || true
	$(PUSH_CMD) quay.io/go-ap/point:$(VERSION)-$(ENV) || true
endif
