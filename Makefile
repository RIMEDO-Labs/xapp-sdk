.PHONY: build
#GO111MODULE=on 

build:
	GOPRIVATE="github.com/onosproject/*" go build -o build/_output/xapp-sdk ./cmd/xapp-sdk

build-tools:=$(shell if [ ! -d "./build/build-tools" ]; then cd build && git clone https://github.com/onosproject/build-tools.git; fi)
include ./build/build-tools/make/onf-common.mk

docker:
	@go mod vendor
	sudo docker build -f build/Dockerfile -t rimedo-labs/xapp-sdk:v0.0.2 . 
	@rm -rf vendor

install-xapp:
	helm install -n riab xapp-sdk ./helm-chart/xapp-sdk --values ./helm-chart/xapp-sdk/values.yaml

delete-xapp:
	-helm uninstall -n riab xapp-sdk

dev: delete-xapp docker install-xapp 