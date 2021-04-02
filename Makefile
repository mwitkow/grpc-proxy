.PHONY: test test-ci install-staticcheck

test:
	go vet ./...
	go test -race -v  -timeout 5m ./...
	CGO_ENABLED=0 staticcheck -f stylish --go 1.14 ./...

install-staticcheck:
	go get honnef.co/go/tools/cmd/staticcheck

test-ci: install-staticcheck | test
