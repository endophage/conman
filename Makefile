# Set an output prefix, which is the local directory if not specified
PREFIX?=$(shell pwd)

PKG = githu.com/endophage/conman

.PHONY: clean all fmt vet lint macapp
.DEFAULT: default

all: AUTHORS clean fmt vet lint macapp

macapp:
	@godep go build -o ${PREFIX}/ConMan.app/Contents/MacOS/conman ./cmd/conman

vet:
	@echo "+ $@"
	@test -z "$$(go tool vet -printf=false . 2>&1 | grep -v vendor | tee /dev/stderr)"

fmt:
	@echo "+ $@"
	@test -z "$$(gofmt -s -l .| grep -v .pb. | grep -v vendor | tee /dev/stderr)"

lint:
	@echo "+ $@"
	@test -z "$$(golint ./... | grep -v .pb. | grep -v vendor | tee /dev/stderr)"

clean:
	@echo "+ $@"
	@rm -rf "$(COVERDIR)"
	@rm -rf "${PREFIX}/bin/notary-server" "${PREFIX}/bin/notary" "${PREFIX}/bin/notary-signer"
