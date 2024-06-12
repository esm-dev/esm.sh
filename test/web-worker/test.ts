import { assertEquals } from "https://deno.land/std@0.220.0/assert/mod.ts";

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
