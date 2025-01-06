import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("typesVersions", async () => {
  const res = await fetch("http://localhost:8080/redux-saga@1.2.0/index.d.ts");
  const dts = await res.text();
  assertStringIncludes(dts, "http://localhost:8080/@redux-saga/core@1.3.0/types/ts4.2/index.d.ts");

  const res2 = await fetch("http://localhost:8080/@redux-saga/core@1.3.0/types/ts4.2/index.d.ts");
  const dts2 = await res2.text();
  assertStringIncludes(dts2, "http://localhost:8080/@redux-saga/types@1.2.1/types/ts3.6/index.d.ts");

  const res3 = await fetch("http://localhost:8080/@babel/types@7.24.7");
  res3.body?.cancel();
  assertEquals(res3.headers.get("x-typescript-types"), "http://localhost:8080/@babel/types@7.24.7/lib/index.d.ts");

  const res4 = await fetch("http://localhost:8080/web-streams-polyfill@3.3.3");
  res4.body?.cancel();
  assertEquals(res4.headers.get("x-typescript-types"), "http://localhost:8080/web-streams-polyfill@3.3.3/dist/types/ts3.6/polyfill.d.ts");
});
