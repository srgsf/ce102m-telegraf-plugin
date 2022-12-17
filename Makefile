APP=ce102m
PROD ?=-s -w
B=$(shell git rev-parse --abbrev-ref HEAD)
BRANCH=$(subst /,-,$(B))
GITREV=$(shell git describe --abbrev=7 --always --tags)
REV=$(GITREV)-$(BRANCH)-$(shell date +%Y-%m-%dT%H:%M:%S)
BUILDER_IMG=$(APP).dist.bin

dist:
	- @mkdir -p dist
	docker build -f Dockerfile.dist --progress=plain -t $(BUILDER_IMG) .
	- @docker rm -f $(BUILDER_IMG) 2>/dev/null || exit 0
	docker run -d --name=$(BUILDER_IMG) $(BUILDER_IMG)
	docker cp $(BUILDER_IMG):/artifacts dist/
	docker rm -f $(BUILDER_IMG)


build: info
	- cd cmd && CGO_ENABLED=0 go build -ldflags "-X github.com/srgsf/ce102m-telegraf-plugin/plugins/inputs/ce102m.version=$(REV) $(PROD)" -o ../dist/$(APP)

build-dev: PROD :=
build-dev: build

info:
	- @echo "revision $(REV)"
clean:
	rm -rf dist

.PHONY: clean build build-dev info dist
