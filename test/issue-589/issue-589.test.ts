import { assertEquals } from "https://deno.land/std@0.180.0/testing/asserts.ts";

Deno.test("issue #589", async () => {
  const { version } = await fetch("http://localhost:8080/status.json").then((
    res,
  ) => res.json());
  const res = await fetch(
    "http://localhost:8080/@types/react@18.0.34/index.d.ts",
    { redirect: "manual" },
  );
  res.body?.cancel();

  assertEquals(res.status, 301);
  assertEquals(
    res.headers.get("location"),
    `http://localhost:8080/v${version}/@types/react@18.0.34/index.d.ts`,
  );
});
