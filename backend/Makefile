# EdgeWatcher backend build.
#
# Terraform manages only the function "shells" (runtime, IAM, routes, env
# vars) and ignores changes to the deployed code (infra/modules/lambda's
# `ignore_changes = [filename, source_code_hash]`); CI ships the actual
# binary via update-function-code (docs/02-infra.md §5). This Makefile
# produces the zips CI uploads.
#
# provided.al2023 + arm64 (05-backend.md §2.1): cross-compile with
# CGO_ENABLED=0 GOOS=linux GOARCH=arm64, and the handler inside the zip
# must be a file named exactly `bootstrap` at the zip's root.

FUNCTIONS := authorizer web-api device-auth device-api
DIST      := dist

.PHONY: build clean fmt vet test lint $(FUNCTIONS)

build: $(FUNCTIONS)

$(FUNCTIONS):
	mkdir -p $(DIST)/$@
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(DIST)/$@/bootstrap ./cmd/$@
	cd $(DIST)/$@ && zip -X -q ../$@.zip bootstrap

fmt:
	gofmt -l .

vet:
	go vet ./...

test:
	go test -race ./...

lint: fmt vet

clean:
	rm -rf $(DIST)
