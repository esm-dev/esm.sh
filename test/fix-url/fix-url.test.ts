import { assertEquals } from "https://deno.land/std@0.180.0/testing/asserts.ts";

const { version } = await fetch("http://localhost:8080/status.json").then(
  (r) => r.json(),
);

Deno.test("Fix wasm URL", async () => {
  const res = await fetch(
    `http://localhost:8080/v${version}/lightningcss-wasm@1.19.0/deno/lightningcss_node.wasm`,
    { redirect: "manual" },
  );
  res.body?.cancel();
  assertEquals(res.status, 301);
  assertEquals(
    res.headers.get("location"),
    "http://localhost:8080/lightningcss-wasm@1.19.0/lightningcss_node.wasm",
  );
});

Deno.test("Fix json URL", async () => {
  const res = await fetch(
    `http://localhost:8080/v${version}/lightningcss-wasm@1.19.0/deno/package.json`,
    { redirect: "manual" },
  );
  res.body?.cancel();
  assertEquals(res.status, 301);
  assertEquals(
    res.headers.get("location"),
    "http://localhost:8080/lightningcss-wasm@1.19.0/package.json",
  );
});
