run/cli/add:
	@go run -tags debug main.go add ${args}

run/cli/tidy:
	@go run -tags debug main.go tidy

run/server: config.json
	@rm -rf .esmd/log
	@rm -rf .esmd/storage
	@go run -tags debug server/esmd/main.go --config=config.json

test/server:
	@./test/bootstrap.ts ${dir}
