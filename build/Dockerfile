FROM onosproject/golang-build:v1.0 as build

ENV GO111MODULE=on

COPY Makefile go.mod go.sum /go/src/github.com/rimedo-labs/xapp-sdk/
COPY cmd/ /go/src/github.com/rimedo-labs/xapp-sdk/cmd/
COPY vendor/ /go/src/github.com/rimedo-labs/xapp-sdk/vendor/
COPY pkg/ /go/src/github.com/rimedo-labs/xapp-sdk/pkg/
COPY build/build-tools /go/src/github.com/rimedo-labs/xapp-sdk/build/build-tools/

RUN cd /go/src/github.com/rimedo-labs/xapp-sdk && GOFLAGS=-mod=vendor make build

FROM alpine:3.11
RUN apk add libc6-compat

USER nobody

COPY --from=build /go/src/github.com/rimedo-labs/xapp-sdk/build/_output/xapp-sdk /usr/local/bin/xapp-sdk

ENTRYPOINT ["xapp-sdk"]