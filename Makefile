.PHONY: test

test:
	go test -timeout 30m -covermode=atomic -coverprofile=coverage.out ./...
