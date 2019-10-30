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

$(shell mkdir -p ./out)

.PHONY: build
build: out/$(BINNAME)

.PHONY: out/$(BINNAME)
out/$(BINNAME):
	GO111MODULE=$(GO111MODULE) go build \
	  -ldflags="-s -w -X $(MODPATH)/pkg/version.version=$(VERSION)" \
	  -a -o $@ cmd/manager/main.go

.PHONY: cicd-build-binary
cicd-build-binary: build

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
