import { assert, assertEquals, assertStringIncludes } from "jsr:@std/assert";

// change the import path to the module you want to test
import {Client} from "http://localhost:8080/@modelcontextprotocol/sdk@1.15.0/client/index.js";

// related issue: https://github.com/esm-dev/esm.sh/issues/1161
Deno.test("testing name", async () => {
  const client = new Client({name: 'test', version: '1.0.0'});
  assert("connect" in client);
  assertEquals(typeof client.connect, "function");
});
