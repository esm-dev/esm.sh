.PHONY: test
test:
	@./test/bootstrap.ts ${dir}

run: config.json
	@rm -rf .esmd/storage
	@go run main.go --config=config.json --debug
