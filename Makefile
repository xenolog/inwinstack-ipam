VERSION_MAJOR  ?= 0
VERSION_MINOR  ?= 7
VERSION_BUILD  ?= 0
VERSION_TSTAMP ?= $(shell date -u +%Y%m%d-%H%M%S)
VERSION_SHA    ?= $(shell git rev-parse --short HEAD)
VERSION ?= v$(VERSION_MAJOR).$(VERSION_MINOR).$(VERSION_BUILD)-mirantis-$(VERSION_TSTAMP)-$(VERSION_SHA)

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

BUILD_FLAGS_DD ?= $(if $(filter $(shell go env GOHOSTOS),darwin),,-d)

BUILD_FLAGS ?= -ldflags="$(BUILD_FLAGS_DD) -s -w -X $(MODPATH)/pkg/version.version=$(VERSION)" -tags netgo -installsuffix netgo

$(shell mkdir -p ./out)

.PHONY: env-info
env-info:
	@echo
	id
	@echo
	@env | grep -i GO
	@echo
	@go version

.PHONY: install-tools
install-tools:
	apk add openssh-client git coreutils

.PHONY: git-config
git-config: install-tools
	git config --global url.'${GERRIT_URL}/'.insteadOf 'https://gerrit.mcp.mirantis.com/a/'

.PHONY: build
build: out/$(BINNAME)

.PHONY: out/$(BINNAME)
out/$(BINNAME): env-info
	CGO_ENABLED=0 go build $(BUILD_FLAGS) -a -o $@ cmd/main.go

.PHONY: cicd-build-binary
cicd-build-binary: install-tools git-config test build

.PHONY: cicd-build-image
cicd-build-image:
	docker build $(CICD_DOCKER_BUILD_ARGS) .

.PHONY: cicd-version
cicd-version:
	@echo $(VERSION_MAJOR).$(VERSION_MINOR).$(VERSION_BUILD)

.PHONY: version
version:
	@echo $(VERSION)

.PHONY: clean
clean:
	rm -rf out/

.PHONY: test
test: env-info
	@gofmt -d  $(shell find . -name '*.go')
	CGO_ENABLED=0 go vet $(BUILD_FLAGS) ./...
	CGO_ENABLED=0 go test $(BUILD_FLAGS) ./...

.PHONY: generate
generate: env-info
	go generate ./...
