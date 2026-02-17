run/cli/init:
	@go run -tags debug main.go init ${args}

run/cli/add:
	@go run -tags debug main.go add ${args}

run/cli/tidy:
	@go run -tags debug main.go tidy

run/cli/dev:
	@go run -tags debug main.go dev ${args} cli/templates/${app}

run/cli/serve:
	@go run -tags debug main.go serve ${args} cli/templates/${app}

run/server: config.json
	@rm -rf .esmd/log
	@rm -rf .esmd/storage
	@go run -tags debug server/esmd/main.go --config=config.json

test/server:
	@./test/bootstrap.ts ${dir}
