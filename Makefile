VERSION_MAJOR  ?= 0
VERSION_MINOR  ?= 7
VERSION_BUILD  ?= 0
VERSION_TSTAMP ?= $(shell date -u +%Y%m%d-%H%M%S)
VERSION_SHA    ?= $(shell git rev-parse --short HEAD)
VERSION ?= v$(VERSION_MAJOR).$(VERSION_MINOR).$(VERSION_BUILD)-$(VERSION_TSTAMP)-$(VERSION_SHA)

GOOS ?= $(shell go env GOOS)

OWNER ?= xenolog
MODPATH ?= $(shell head -n1 go.mod | grep module | awk '{print $$2}')
BINNAME ?= inwinstack-ipam

$(shell mkdir -p ./out)

.PHONY: build
build: out/$(BINNAME)

.PHONY: out/$(BINNAME)
out/$(BINNAME):
	GOOS=$(GOOS) go build \
	  -ldflags="-s -w -X $(MODPATH)/pkg/version.version=$(VERSION)" \
	  -a -o $@ cmd/main.go

.PHONY: test
test:
	./hack/test-go.sh

.PHONY: build_image
build_image:
	docker build -t $(OWNER)/inwinstack-ipam:$(VERSION) .

.PHONY: push_image
push_image:
	docker push $(OWNER)/inwinstack-ipam:$(VERSION)

.PHONY: clean
clean:
	rm -rf out/

