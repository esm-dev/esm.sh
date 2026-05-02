# esm.sh

A _no-build_ JavaScript CDN for modern web development.

## Project Structure

- `cli/`: Command-line interface (releases as the npm `esm.sh` CLI).
- `internal/`: Shared Go packages reused by both the server and the CLI—NPM resolution, storage, build helpers, and related utilities.
- `server/`: Main HTTP service: request handling, bundling, and CDN behavior.
- `test/`: Deno-based integration suites; each subdirectory exercises imports against a running server (`test/.template` is the scaffold for new cases).
- `web/`: Landing site and docs: static assets plus Go handlers that serve them alongside the CDN.

## Running the Server in Debug Mode

```bash
make run/server
```

Then you can import `React` from "http://localhost:8080/react"

## Running Server Integration Tests

We use [Deno](https://deno.land) to run all the integration testing cases. Make sure you have Deno installed on your computer.

```bash
# Run all tests
make test/server

# Run a specific test
make test/server dir=react-18
```

To add a new integration test case, copy the [test/.template](./test/.template) directory and rename it to your case name.

```bash
# copy the testing template
cp -r test/.template test/test-case-name
# edit the test code
vi test/test-case-name/test.ts
# run the test
make test/server dir=test-case-name
```
