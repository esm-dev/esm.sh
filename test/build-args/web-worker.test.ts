import { assert, assertEquals, assertStringIncludes } from "jsr:@std/assert";

import workerFactory from "http://localhost:8080/xxhash-wasm@1.0.2?worker";

const inject = `
self.onmessage = (e) => {
  // variable '$module' is the xxhash-wasm module
  $module.default().then(hasher => {
    self.postMessage(hasher.h64ToString(e.data));
  })
}
`;

Deno.test("web-worker (legacy api)", async () => {
  const worker = workerFactory(inject);
  const hashText = await new Promise((resolve, reject) => {
    const t = setTimeout(() => {
      reject("timeout");
    }, 1000);
    worker.addEventListener("message", (e) => {
      clearTimeout(t);
      resolve(e.data);
    });
    worker.postMessage("The string that is being hashed");
  });
  assertEquals(hashText, "502b0c5fc4a5704c");
  worker.terminate();
});

Deno.test("web-worker", async () => {
  const worker = workerFactory({ inject, name: "xxhash-wasm" });
  const hashText = await new Promise((resolve, reject) => {
    const t = setTimeout(() => {
      reject("timeout");
    }, 1000);
    worker.addEventListener("message", (e) => {
      clearTimeout(t);
      resolve(e.data);
    });
    worker.postMessage("The string that is being hashed");
  });
  assertEquals(hashText, "502b0c5fc4a5704c");
  worker.terminate();
});

Deno.test("web-worker output ignores forwarded origin", async () => {
  const entryUrl = "http://localhost:8080/xxhash-wasm@1.0.2?target=es2022";
  const entryRes = await fetch(entryUrl);
  entryRes.body?.cancel();
  const buildPath = entryRes.headers.get("x-esm-path");
  assert(buildPath);

  for (const url of [
    entryUrl + "&worker&origin-regression",
    new URL(buildPath + "?worker&origin-regression", entryUrl),
  ]) {
    const res = await fetch(url, {
      headers: {
        "X-Real-Origin": `https://attacker.invalid" } = options; globalThis.PWNED = true; const { z = "`,
      },
    });
    const code = await res.text();
    assertEquals(res.status, 200);
    assertStringIncludes(code, "new URL(\"/");
    assertStringIncludes(code, "JSON.stringify(moduleUrl)");
    assert(!code.includes("attacker.invalid"));
    assert(!code.includes("globalThis.PWNED"));
  }
});
