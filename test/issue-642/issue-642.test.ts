import {
  assertEquals,
  assertStringIncludes,
} from "https://deno.land/std@0.180.0/testing/asserts.ts";

const { version } = await fetch("http://localhost:8080/status.json").then(
  (r) => r.json(),
);

Deno.test("issue #642", async () => {
  const res = await fetch(
    `http://localhost:8080/v${version}/async-mutex@0.4.0/lib/Mutex.d.ts`,
    { redirect: "manual" },
  );
  assertEquals(res.status, 200);
  assertEquals(
    res.headers.get("content-type")!,
    "application/typescript; charset=utf-8",
  );
  assertStringIncludes(await res.text(), "./MutexInterface.d.ts");
});
