VERSION_MAJOR ?= 0
VERSION_MINOR ?= 7
VERSION_BUILD ?= 0
VERSION ?= v$(VERSION_MAJOR).$(VERSION_MINOR).$(VERSION_BUILD)

ORG := github.com
OWNER := inwinstack
REPOPATH ?= $(ORG)/$(OWNER)/ipam

$(shell mkdir -p ./out)

.PHONY: build
build: out/controller

.PHONY: out/controller
out/controller: 
	CGO_ENABLED=0 go build \
	  -ldflags="-d -s -w -X $(REPOPATH)/pkg/version.version=$(VERSION)" \
	  -a -tags netgo -installsuffix netgo  -o $@ cmd/main.go

.PHONY: test
test:
	./hack/test-go.sh

.PHONY: build_image
build_image:
	docker build -t $(OWNER)/ipam:$(VERSION) .

.PHONY: push_image
push_image:
	docker push $(OWNER)/ipam:$(VERSION)

.PHONY: clean
clean:
	rm -rf out/

