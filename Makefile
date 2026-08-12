IMAGE ?= ghcr.io/adityathebe/mc-demo
TAG ?= dev

.PHONY: build test run docker-build deploy

build:
	go build -o bin/mc-demo-app ./cmd/server

test:
	go test ./...

run:
	go run ./cmd/server

docker-build:
	docker build --build-arg COMMIT=$$(git rev-parse --short HEAD 2>/dev/null || echo unknown) -t $(IMAGE):$(TAG) .

deploy:
	kubectl apply -k deploy/kubernetes
