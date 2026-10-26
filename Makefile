.PHONY: test

# Unpack the posix fixture the same way .drone.yml does before the test suite.
test:
	tar -xf posix/fixtures.tar -C /
	go test -timeout 30m -covermode=atomic -coverprofile=coverage.out ./...
