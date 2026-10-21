.PHONY: test

test:
	tar -xf posix/fixtures.tar -C /
	go test -timeout 30m -cover -coverprofile=coverage.out ./...
