SHELL ?= /bin/bash

.DEFAULT_GOAL := push-images

################################################################################
# Version details                                                              #
################################################################################

# This will reliably return the short SHA1 of HEAD or, if the working directory
# is dirty, will return that + "-dirty"
GIT_VERSION = $(shell git describe --always --abbrev=7 --dirty --match=NeVeRmAtCh)

################################################################################
# Containerized development environment-- or lack thereof                      #
################################################################################

ifneq ($(SKIP_DOCKER),true)
	PROJECT_ROOT := $(dir $(realpath $(firstword $(MAKEFILE_LIST))))
	GO_DEV_IMAGE := brigadecore/go-tools:v0.8.0

	GO_DOCKER_CMD := docker run \
		-it \
		--rm \
		-e SKIP_DOCKER=true \
		-e GOCACHE=/workspaces/brigade-github-gateway/.gocache \
		-v $(PROJECT_ROOT):/workspaces/brigade-github-gateway \
		-w /workspaces/brigade-github-gateway \
		$(GO_DEV_IMAGE)

	HELM_IMAGE := brigadecore/helm-tools:v0.4.0

	HELM_DOCKER_CMD := docker run \
	  -it \
		--rm \
		-e SKIP_DOCKER=true \
		-e HELM_PASSWORD=$${HELM_PASSWORD} \
		-v $(PROJECT_ROOT):/workspaces/brigade-github-gateway \
		-w /workspaces/brigade-github-gateway \
		$(HELM_IMAGE)
endif

################################################################################
# Docker images and charts we build and publish                                #
################################################################################

ifdef DOCKER_REGISTRY
	DOCKER_REGISTRY := $(DOCKER_REGISTRY)/
endif

ifdef DOCKER_ORG
	DOCKER_ORG := $(DOCKER_ORG)/
endif

DOCKER_IMAGE_PREFIX := $(DOCKER_REGISTRY)$(DOCKER_ORG)brigade-github-gateway-

ifdef HELM_REGISTRY
	HELM_REGISTRY := $(HELM_REGISTRY)/
endif

ifdef HELM_ORG
	HELM_ORG := $(HELM_ORG)/
endif

HELM_CHART_PREFIX := $(HELM_REGISTRY)$(HELM_ORG)

ifdef VERSION
	MUTABLE_DOCKER_TAG := latest
else
	VERSION            := $(GIT_VERSION)
	MUTABLE_DOCKER_TAG := edge
endif

IMMUTABLE_DOCKER_TAG := $(VERSION)

################################################################################
# Tests                                                                        #
################################################################################

.PHONY: lint
lint:
	$(GO_DOCKER_CMD) golangci-lint run --config golangci.yaml

.PHONY: test-unit
test-unit:
	$(GO_DOCKER_CMD) go test \
		-v \
		-timeout=60s \
		-race \
		-coverprofile=coverage.txt \
		-covermode=atomic \
		./...

.PHONY: lint-chart
lint-chart:
	$(HELM_DOCKER_CMD) sh -c ' \
		cd charts/brigade-github-gateway && \
		helm dep up && \
		helm lint . \
	'

################################################################################
# Upload Code Coverage Reports                                                 #
################################################################################

.PHONY: upload-code-coverage
upload-code-coverage:
	$(GO_DOCKER_CMD) codecov

################################################################################
# Image security                                                               #
################################################################################

.PHONY: scan-%
scan-%:
	grype $(DOCKER_IMAGE_PREFIX)$*:$(IMMUTABLE_DOCKER_TAG) -f medium

.PHONY: generate-sbom-%
generate-sbom-%:
	syft $(DOCKER_IMAGE_PREFIX)$*:$(IMMUTABLE_DOCKER_TAG) \
		-o spdx-json \
		--file ./artifacts/brigade-github-gateway-$*-$(VERSION)-SBOM.json

.PHONY: publish-sbom-%
publish-sbom-%: generate-sbom-%
	ghr \
		-u $(GITHUB_ORG) \
		-r $(GITHUB_REPO) \
		-c $$(git rev-parse HEAD) \
		-t $${GITHUB_TOKEN} \
		-n ${VERSION} \
		${VERSION} ./artifacts/brigade-github-gateway-$*-$(VERSION)-SBOM.json

################################################################################
# Publish                                                                      #
################################################################################

.PHONY: publish
publish: push-images publish-chart

.PHONY: push-images
push-images: push-receiver push-monitor

.PHONY: push-%
push-%:
	docker buildx build \
		-f $*/Dockerfile \
		-t $(DOCKER_IMAGE_PREFIX)$*:$(IMMUTABLE_DOCKER_TAG) \
		-t $(DOCKER_IMAGE_PREFIX)$*:$(MUTABLE_DOCKER_TAG) \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(GIT_VERSION) \
		--platform linux/amd64,linux/arm64 \
		--push \
		.

.PHONY: sign-%
sign-%:
	docker pull $(DOCKER_IMAGE_PREFIX)$*:$(IMMUTABLE_DOCKER_TAG)
	docker pull $(DOCKER_IMAGE_PREFIX)$*:$(MUTABLE_DOCKER_TAG)
	docker trust sign $(DOCKER_IMAGE_PREFIX)$*:$(IMMUTABLE_DOCKER_TAG)
	docker trust sign $(DOCKER_IMAGE_PREFIX)$*:$(MUTABLE_DOCKER_TAG)
	docker trust inspect --pretty $(DOCKER_IMAGE_PREFIX)$*:$(IMMUTABLE_DOCKER_TAG)
	docker trust inspect --pretty $(DOCKER_IMAGE_PREFIX)$*:$(MUTABLE_DOCKER_TAG)

.PHONY: publish-chart
publish-chart:
	$(HELM_DOCKER_CMD) sh	-c ' \
		helm registry login $(HELM_REGISTRY) -u $(HELM_USERNAME) -p $${HELM_PASSWORD} && \
		cd charts/brigade-github-gateway && \
		helm dep up && \
		helm package . --version $(VERSION) --app-version $(VERSION) && \
		helm push brigade-github-gateway-$(VERSION).tgz oci://$(HELM_REGISTRY)$(HELM_ORG) \
	'

################################################################################
# Targets to facilitate hacking on this gateway                                #
################################################################################

.PHONY: hack-kind-up
hack-kind-up:
	ctlptl apply -f hack/kind/cluster.yaml
	HELM_EXPERIMENTAL_OCI=1 helm upgrade brigade \
		oci://ghcr.io/brigadecore/brigade \
		--version v2.4.0 \
		--install \
		--create-namespace \
		--namespace brigade \
		--set apiserver.rootUser.password='F00Bar!!!' \
		--set apiserver.service.type=NodePort \
		--set apiserver.service.nodePort=31600 \
		--wait \
		--timeout 300s

.PHONY: hack-kind-down
hack-kind-down:
	ctlptl delete -f hack/kind/cluster.yaml
