.PHONY: test fixtures

test: fixtures
	go test -mod=readonly -coverprofile=coverage.out ./...

fixtures:
	tar -xof posix/fixtures.tar -C /
