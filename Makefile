dev: config.json
	@rm -rf .esmd/log
	@rm -rf .esmd/storage
	@rm -rf .esmd/esm.db
	@go run -tags debug main.go --config=config.json

dev/cli/serve:
	@go run -tags debug cli/cmd/main.go serve cli/cmd/demo/${app}

dev/cli/init:
	@go run -tags debug cli/cmd/main.go init

dev/cli/im/add:
	@go run -tags debug cli/cmd/main.go im add ${package}

dev/cli/im/update:
	@go run -tags debug cli/cmd/main.go im update ${package}

.PHONY: test
test:
	@./test/bootstrap.ts ${dir}
