debug/cli/dev:
	@go run -tags debug main.go dev cli/demo/${app}

debug/cli/serve:
	@go run -tags debug main.go serve cli/demo/${app}

debug/cli/init:
	@go run -tags debug main.go init

debug/cli/add:
	@go run -tags debug main.go add ${package}

debug/cli/importmap:
	@go run -tags debug main.go importmap ${command}

debug/server: config.json
	@rm -rf .esmd/log
	@rm -rf .esmd/storage
	@rm -rf .esmd/esm.db
	@go run -tags debug server/esmd/main.go --config=config.json

test/server:
	@./test/bootstrap.ts ${dir}
