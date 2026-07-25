import { assert, assertEquals, assertStringIncludes } from "jsr:@std/assert";

const attack = `https://esm.sh" } = options; globalThis.PWNED=1; const { z = "`;
const headers = {
  "x-real-origin": attack,
  "cf-visitor": `{"scheme":"https"}`,
};
const browserHeaders = { "user-agent": "Mozilla/5.0" };

Deno.test("request origin does not affect cacheable responses", async () => {
  const workerUrl = "http://localhost:8080/xxhash-wasm@1.0.2?worker";
  const poisonedWorker = await fetch(workerUrl, { headers }).then((res) =>
    res.text()
  );
  const worker = await fetch(workerUrl).then((res) => res.text());
  const otherHostWorker = await fetch(
    workerUrl.replace("localhost", "127.0.0.1"),
  ).then((res) => res.text());
  assertEquals(poisonedWorker, worker);
  assertEquals(otherHostWorker, worker);
  assert(!worker.includes(attack));
  assertStringIncludes(worker, `const moduleUrl = "http://localhost:8080/`);

  const targetWorkerUrl =
    "http://localhost:8080/xxhash-wasm@1.0.2/es2022/xxhash-wasm.mjs?worker";
  const targetWorker = await fetch(targetWorkerUrl).then((res) => res.text());
  const targetModuleUrl = targetWorkerUrl.replace("?worker", "");
  const metaUrl =
    "http://localhost:8080/xxhash-wasm@1.0.2?meta&target=es2022";
  const moduleBefore = await fetch(targetModuleUrl).then((res) => res.text());
  const metaBefore = await fetch(metaUrl).then((res) => res.text());
  const poisonedTargetWorker = await fetch(targetWorkerUrl, { headers }).then(
    (res) => res.text(),
  );
  assertEquals(poisonedTargetWorker, targetWorker);
  assertEquals(
    await fetch(targetModuleUrl, { headers }).then((res) => res.text()),
    moduleBefore,
  );
  assertEquals(
    await fetch(metaUrl, { headers }).then((res) => res.text()),
    metaBefore,
  );
  assert(JSON.parse(metaBefore).integrity.startsWith("sha384-"));
  assert(!targetWorker.includes(attack));
  assertStringIncludes(
    targetWorker,
    `const moduleUrl = "http://localhost:8080/`,
  );

  const poisonedIndexRes = await fetch("http://localhost:8080/", {
    headers: { ...browserHeaders, ...headers },
  });
  assertStringIncludes(
    poisonedIndexRes.headers.get("content-type") ?? "",
    "text/html",
  );
  const poisonedIndex = await poisonedIndexRes.text();
  const index = await fetch("http://localhost:8080/", {
    headers: browserHeaders,
  }).then((res) => res.text());
  assertEquals(poisonedIndex, index);
  assert(!index.includes(attack));
  assertStringIncludes(index, "A _no-build_ JavaScript CDN");

  const entry = "http://localhost:8080/@octokit-next/types-rest-api@2.5.0";
  const entryRes = await fetch(entry, { headers });
  entryRes.body?.cancel();
  const dtsUrl = entryRes.headers.get("x-typescript-types");
  assert(dtsUrl);
  assertEquals(dtsUrl, `${entry}/index.d.ts`);
  const poisonedDts = await fetch(dtsUrl, { headers }).then((res) =>
    res.text()
  );
  const dts = await fetch(dtsUrl).then((res) => res.text());
  assertEquals(poisonedDts, dts);
  assert(!dts.includes(attack));
  assertStringIncludes(dts, "http://localhost:8080/");
});
