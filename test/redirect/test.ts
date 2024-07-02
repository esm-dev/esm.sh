import { assert, assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("redirects", async () => {
  const res = await fetch("http://localhost:8080/react", { redirect: "manual" });
  res.body?.cancel();
  assertEquals(res.status, 302);
  assertStringIncludes(res.headers.get("location")!, "http://localhost:8080/react@");

  const res2 = await fetch("http://localhost:8080/react@18", { redirect: "manual" });
  res2.body?.cancel();
  assertEquals(res2.status, 302);
  assertStringIncludes(res2.headers.get("location")!, "http://localhost:8080/react@18.");

  const res3 = await fetch("http://localhost:8080/gh/microsoft/tslib", { redirect: "manual" });
  res3.body?.cancel();
  assertEquals(res3.status, 302);
  assertStringIncludes(res3.headers.get("location")!, "http://localhost:8080/gh/microsoft/tslib@");

  const res4 = await fetch("http://localhost:8080/jsr/@std/encoding", { redirect: "manual" });
  res4.body?.cancel();
  assertEquals(res4.status, 302);
  assertStringIncludes(res4.headers.get("location")!, "http://localhost:8080/jsr/@std/encoding@");

  // doesn't redirect if the version is fully specified or caret range
  const res5 = await fetch("http://localhost:8080/react@18.3.1", { redirect: "manual" });
  res5.body?.cancel();
  assertEquals(res5.status, 200);
  const res6 = await fetch("http://localhost:8080/react@^18.3.1", { redirect: "manual" });
  res6.body?.cancel();
  assertEquals(res6.status, 200);
  const res7 = await fetch("http://localhost:8080/jsr/@std/encoding@1.0.0", { redirect: "manual" });
  res7.body?.cancel();
  assertEquals(res7.status, 200);
});
