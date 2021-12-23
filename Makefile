.PHONY: build

build:
	GOPRIVATE="github.com/onosproject/*" go build -o build/_output/xapp-sdk ./cmd

build-tools:=$(shell if [ ! -d "./build/build-tools" ]; then cd build && git clone https://github.com/onosproject/build-tools.git; fi)
include ./build/build-tools/make/onf-common.mk


docker:
	@go mod vendor
	sudo docker build -f build/Dockerfile -t xapp-sdk:latest .
	@rm -rf vendor

install-xapp:
	kubectl -n riab apply -f deployment/deployment.yaml
	kubectl -n riab apply -f deployment/service.yaml

delete-xapp:
	kubectl -n riab delete -f deployment/deployment.yaml
	kubectl -n riab delete -f deployment/service.yaml