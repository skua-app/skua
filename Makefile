.PHONY: check check-changelog build clean

# Run all quality checks: CHANGELOG consistency + Go (gofmt, go vet, golangci-lint, go test) + frontend (svelte-check, tsc, eslint, prettier).
# The changelog check runs first because it takes milliseconds and needs no toolchain.
check: check-changelog
	$(MAKE) -C backend check
	cd frontend && npm run check && npm run lint && npx prettier --check . && npm run test

# Cross-check CHANGELOG headings against the comparison-link definitions and
# the local git tags. Degrades to link-only checking when tags are not present
# (shallow / --no-tags checkout) and says so. Never touches the network.
check-changelog:
	@./scripts/check-changelog.sh

build:
	docker buildx build --load -t skua:dev .

clean:
	$(MAKE) -C backend clean
