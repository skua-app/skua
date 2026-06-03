.PHONY: check build clean

# Run all quality checks: Go (gofmt, go vet, golangci-lint, go test) + frontend (svelte-check, tsc, eslint, prettier).
check:
	$(MAKE) -C backend check
	cd frontend && npm run check && npm run lint && npx prettier --check . && npm run test

build:
	docker buildx build --load -t skua:dev .

clean:
	$(MAKE) -C backend clean
