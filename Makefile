.PHONY: test
test:
	@./test/bootstrap.ts --clean ${dir}

run: config.json
	@rm -rf .esmd/storage
	@go run main.go --debug --config=config.json
