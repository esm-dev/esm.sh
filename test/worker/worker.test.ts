import { assertEquals } from "https://deno.land/std@0.210.0/testing/asserts.ts";

import workerFactory from "http://localhost:8080/xxhash-wasm@1.0.2?worker";

const workerInject = `
self.onmessage = (e) => {
  // variable 'E' is the xxhash-wasm module default export
  E().then(hasher => {
    self.postMessage(hasher.h64ToString(e.data));
  })
}
`;

Deno.test("?worker", async () => {
  const worker = workerFactory(workerInject);
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
  worker.terminate();
  assertEquals(hashText, "502b0c5fc4a5704c");
});
