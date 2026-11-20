import { assert, assertEquals, assertObjectMatch } from "jsr:@std/assert";

Deno.test("issue #1499 - load MapLibre's relative module worker", async () => {
  const entry = await fetch(
    "http://localhost:8080/maplibre-gl@6.16.0?target=es2022",
  );
  await entry.body?.cancel();
  assertEquals(entry.status, 200);
  const modulePath = entry.headers.get("x-esm-path");
  assert(modulePath);

  const workerUrl = new URL(
    "./maplibre-gl-worker.mjs",
    new URL(modulePath, entry.url),
  );
  workerUrl.searchParams.set("v", "6.16.0");
  const res = await fetch(workerUrl);
  await res.body?.cancel();
  assertEquals(res.status, 200);
  assertEquals(
    res.headers.get("content-type"),
    "application/javascript; charset=utf-8",
  );
  assertEquals(new URL(res.url).search, workerUrl.search);

  const worker = new Worker(workerUrl.href, { type: "module" });
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    const reply = await new Promise<Record<string, unknown>>(
      (resolve, reject) => {
        timer = setTimeout(
          () => reject(new Error("Worker did not respond")),
          10_000,
        );
        worker.onmessage = (event) => resolve(event.data);
        worker.onerror = (event) => {
          event.preventDefault();
          reject(new Error(event.message));
        };
        worker.postMessage({
          id: "test",
          type: "PI",
          origin: workerUrl.origin,
          sourceMapId: "test",
        });
      },
    );
    assertObjectMatch(reply, { id: "test", type: "<response>", error: null });
  } finally {
    clearTimeout(timer);
    worker.terminate();
  }
});
