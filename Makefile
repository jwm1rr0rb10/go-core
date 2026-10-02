GO ?= go
PKGS := ./...

.PHONY: all fmt vet lint test race cover bench fuzz tidy check

all: check

fmt:
	gofmt -s -w .

vet:
	$(GO) vet $(PKGS)

lint:
	golangci-lint run $(PKGS)

test:
	$(GO) test -count=1 $(PKGS)

race:
	$(GO) test -race -count=1 $(PKGS)

cover:
	$(GO) test -race -count=1 -coverprofile=coverage.out -covermode=atomic $(PKGS)
	$(GO) tool cover -func=coverage.out | tail -1

bench:
	$(GO) test -run=^$$ -bench=. -benchmem $(PKGS)

fuzz:
	$(GO) test -run=^$$ -fuzz=FuzzFlattenDeep -fuzztime=30s ./array
	$(GO) test -run=^$$ -fuzz=FuzzParse -fuzztime=30s ./api/jwt
	$(GO) test -run=^$$ -fuzz=FuzzParse -fuzztime=30s ./uuid

tidy:
	$(GO) mod tidy

check: vet race
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed:"; gofmt -l .; exit 1)
