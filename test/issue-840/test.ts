import { assertExists } from "jsr:@std/assert";

import * as utils from "http://localhost:8080/@walletconnect/jsonrpc-utils@1.0.6";
import * as _ from "http://localhost:8080/lodash@4.17.21";

// https://github.com/esm-dev/esm.sh/issues/840
Deno.test("issue-840", () => {
  assertExists(utils.IJsonRpcProvider);
  assertExists(utils.isReactNative);
  assertExists(_.merge);
});
