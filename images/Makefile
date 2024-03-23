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

BUILD_CMD=buildah bud
RUN_CMD=podman run
TAG_CMD=podman tag
PUSH_CMD=podman push

ifneq ($(STORAGE),all)
	TAG=$(ENV)-$(STORAGE)
endif

.PHONY: clean images cert build builder push

$(APP_HOSTNAME).pem:
	./gen-certs.sh point

cert: $(APP_HOSTNAME).pem

clean:
	@-$(RM) $(APP_HOSTNAME).{key,crt,pem}

builder:
	./build.sh .. point/builder

build:
	ENV=$(ENV) VERSION=$(VERSION) STORAGE=$(STORAGE) PORT=$(PORT) APP_HOSTNAME=$(APP_HOSTNAME) ./image.sh $(APP_HOSTNAME)/app:$(TAG)

push: build
	$(TAG_CMD) $(APP_HOSTNAME)/app:$(TAG) quay.io/go-ap/point:$(TAG)
	$(PUSH_CMD) quay.io/go-ap/point:$(TAG)
ifeq ($(TAG),dev)
	$(TAG_CMD) $(APP_HOSTNAME)/app:$(TAG) quay.io/go-ap/point:latest || true
	$(PUSH_CMD) quay.io/go-ap/point:latest || true
endif
ifneq ($(VERSION),)
	$(TAG_CMD) $(APP_HOSTNAME)/app:$(ENV) quay.io/go-ap/point:$(VERSION)-$(TAG) || true
	$(PUSH_CMD) quay.io/go-ap/point:$(VERSION)-$(TAG) || true
endif
