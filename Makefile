IMAGE ?= docker.io/imroc/podcidr-controller
TAG ?= latest
PLATFORMS ?= linux/amd64,linux/arm64

.PHONY: build
build:
	go build -o bin/podcidr-controller .

.PHONY: test
test:
	go test ./... -v

.PHONY: docker-build
docker-build:
	docker buildx build --platform $(PLATFORMS) -t $(IMAGE):$(TAG) --push .

.PHONY: docker-build-local
docker-build-local:
	docker build -t $(IMAGE):$(TAG) .

.PHONY: lint
lint:
	golangci-lint run

.PHONY: clean
clean:
	rm -rf bin/
