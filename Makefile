.PHONY: cli
cli:
	@go run cli/cmd/main.go

.PHONY: test
test:
	@./test/bootstrap.ts ${dir}

run: config.json
	@rm -rf .esmd/storage
	@go run main.go --config=config.json --debug
