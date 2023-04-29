import {
  assert,
  assertEquals,
} from "https://deno.land/std@0.180.0/testing/asserts.ts";

Deno.test("issue #589", async () => {
  const { version } = await fetch("http://localhost:8080/status.json").then((
    res,
  ) => res.json());
  const res = await fetch(
    "http://localhost:8080/@types/react@^18/index.d.ts",
    { redirect: "manual" },
  );
  res.body?.cancel();

  assertEquals(res.status, 302);
  assert(
    res.headers.get("location")!.startsWith(
      `http://localhost:8080/v${version}/@types/react@18.`,
    ),
  );
});
