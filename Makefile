dev/cli:
	@go run -tags debug cli/cmd/main.go serve cli/cmd/demo/${app}

dev: config.json
	@rm -rf .esmd/storage
	@rm -rf .esmd/esm.db
	@go run -tags debug main.go --config=config.json

.PHONY: test
test:
	@./test/bootstrap.ts ${dir}
