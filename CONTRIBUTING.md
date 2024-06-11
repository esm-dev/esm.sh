# Contributing to esm.sh

Welcome, and thank you for taking time in contributing to esm.sh project!

## Development Setup

You will need [Golang](https://golang.org/)(1.21+) and [Deno](https://deno.land)(1.40+) installed on a Linux or macOS machine.

1. Fork this repository to your own GitHub account.
2. Clone the repository to your local device.
3. Create a new branch (`git checkout -b BRANCH_NAME`).
4. Change code then run tests
5. Push your branch to Github after **all tests passed**.
6. Make a [pull request](https://github.com/esm-dev/esm.sh/pulls).
7. Merge to master branch by our maintainers.

## Configration

Create a `config.json` file in the project root directory following the example below:

```jsonc
// config.json
{
  "port": 8080,
  "workDir": ".esmd",
  "npmRegistry": "https://registry.npmjs.org/", // change to your own registry if needed
  "npmToken": "xxxxxx" // remove this line if you don't need a token
}
```

More server options please check [config.exmaple.jsonc](./config.example.jsonc).

## Running the Server from Source Code

```bash
go run main.go --debug
```

Then you can import `React` from "http://localhost:8080/react"

## Running Integration Tests

We use [Deno](https://deno.land) to run all the integration testing cases. Make sure you have Deno installed on your machine.

```bash
# Run all tests
./test/bootstrap.ts

# Run a test for a specific case (directory name)
./test/bootstrap.ts preact

# Run tests with `clean` option (purge previous builds)
./test/bootstrap.ts --clean
```

To add a new integration test case, copy the [test/_template](./test/_template) directory and rename it to your case name.

```bash
cp -r test/_template test/case_name
nvim test/case_name/test.ts
./test/bootstrap.ts case_name
```
