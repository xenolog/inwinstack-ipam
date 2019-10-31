VERSION_MAJOR  ?= 0
VERSION_MINOR  ?= 7
VERSION_BUILD  ?= 0
VERSION_TSTAMP ?= $(shell date -u +%Y%m%d-%H%M%S)
VERSION_SHA    ?= $(shell git rev-parse --short HEAD)
VERSION ?= v$(VERSION_MAJOR).$(VERSION_MINOR).$(VERSION_BUILD)-mirantis-$(VERSION_TSTAMP)-$(VERSION_SHA)

GO111MODULE ?= on

MODPATH ?= $(shell head -n1 go.mod | grep module | awk '{print $$2}')
BINNAME ?= inwinstack-ipam

IMAGE_NAME ?= bm/$(BINNAME)
IMAGE_TAG ?= $(VERSION)
CICD_DOCKER_BUILD_ARGS = --tag $(IMAGE_NAME):$(IMAGE_TAG)
ifdef DOCKER_BUILD_ARGS
	CICD_DOCKER_BUILD_ARGS += $(foreach arg,$(DOCKER_BUILD_ARGS),--build-arg $(arg))
endif
ifdef DOCKER_BUILD_LABELS
	CICD_DOCKER_BUILD_ARGS += --label "$(DOCKER_BUILD_LABELS)"
endif
ifdef DOCKER_FILE
	CICD_DOCKER_BUILD_ARGS += --file "$(DOCKER_FILE)"
endif
GERRIT_URL ?= ssh://gerrit.mcp.mirantis.net:29418

$(shell mkdir -p ./out)

.PHONY: install-tools
install-tools:
	apk add openssh-client git coreutils

.PHONY: git-config
git-config: install-tools
	git config --global url.'${GERRIT_URL}/'.insteadOf 'https://gerrit.mcp.mirantis.com/a/'

.PHONY: build
build: out/$(BINNAME)

.PHONY: out/$(BINNAME)
out/$(BINNAME):
	CGO_ENABLED=0 GO111MODULE=$(GO111MODULE) go build \
	  -ldflags="-s -w -X $(MODPATH)/pkg/version.version=$(VERSION)" \
	  -a -o $@ cmd/main.go

.PHONY: cicd-build-binary
cicd-build-binary: install-tools git-config build

.PHONY: cicd-build-image
cicd-build-image:
	docker build $(CICD_DOCKER_BUILD_ARGS) .

.PHONY: cicd-version
cicd-version:
	@echo $(VERSION_MAJOR).$(VERSION_MINOR).$(VERSION_BUILD)

.PHONY: version
version:
	@echo $(VERSION)

.PHONY: test
test:
	./hack/test-go.sh

.PHONY: clean
clean:
	rm -rf out/
