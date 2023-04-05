# Contributing to esm.sh

Welcome, and thank you for taking time in contributing to esm.sh project! ♥️

## Development Setup

You will need [Golang](https://golang.org/) 1.18+.

1. Fork this repository to your own GitHub account.
2. Clone the repository to your local device.
3. Create a new branch `git checkout -b BRANCH_NAME`.
4. Change code then run the testings.
5. [Push your branch to Github after all tests passed.](#Testing)
6. Make a [pull request](https://github.com/esm-dev/esm.sh/pulls).
7. Merge to master branch by our maintainers.

## Configration

To configure the server, you need to create a `config.json` file then pass it to the server bootstrap command. For example:

```jsonc
// config.json
{
  "port": 8080,
  "workDir": ".esmd",
  "npmRegistry": "https://npmjs.org/registry",
  "npmToken": "xxxxxx"
}
```

You can find all the server options in [config.exmaple.jsonc](./config.example.jsonc). (**Note**: the `config.example.jsonc` is not a valid JSON file, it's a JSONC file.)

## Run the sever in development mode

```bash
go run main.go --config=config.json --dev
```

Then you can import `React` from http://localhost:8080/react

## Run testings

```bash
# Run all tests
./test/bootstrap.sh
# Run tests for a specific module
./test/bootstrap.sh preact
```

All tests are written in Deno, you can find them in [test/](./test) directory.

## Code of Conduct

All contributors are expected to follow our [Code of Conduct](CODE_OF_CONDUCT.md).
