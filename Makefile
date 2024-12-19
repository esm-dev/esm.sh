cli/run:
	@DEBUG=1 go run cli/cmd/main.go run cli/cmd/demo/${app}

serv: config.json
	@rm -rf .esmd/storage
	@go run main.go --config=config.json --debug

.PHONY: test
test:
	@./test/bootstrap.ts ${dir}
