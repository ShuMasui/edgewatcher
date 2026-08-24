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

.PHONY: build clean fmt vet test test-integration lint $(FUNCTIONS)

build: $(FUNCTIONS)

$(FUNCTIONS):
	mkdir -p $(DIST)/$@
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(DIST)/$@/bootstrap ./cmd/$@
	cd $(DIST)/$@ && zip -X -q ../$@.zip bootstrap

fmt:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "unformatted:"; echo "$$out"; exit 1; fi

vet:
	go vet ./...

test:
	go test -race ./...

# Requires a DynamoDB Local instance reachable at DYNAMO_ENDPOINT (defaults
# to http://localhost:8000, matching amazon/dynamodb-local's default port).
# No AWS account or credentials are ever used — internal/store's harness
# supplies static dummy credentials.
#
#   docker run -d --name ew-dynamodb-local -p 8000:8000 amazon/dynamodb-local
#   make test-integration
#   docker rm -f ew-dynamodb-local
test-integration:
	DYNAMO_ENDPOINT=$${DYNAMO_ENDPOINT:-http://localhost:8000} go test -race -tags=integration ./internal/store/...

lint: fmt vet

clean:
	rm -rf $(DIST)
