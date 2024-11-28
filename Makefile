
run:
	@rm -rf .esmd/storage
	@go run main.go --debug

.PHONY: test
test:
	@./test/bootstrap.ts --clean
