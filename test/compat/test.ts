import { assertStringIncludes } from "jsr:@std/assert";

Deno.test("HeadlessChrome", async () => {
  const res = await fetch("http://localhost:8080/react@18.2.0", {
    headers: {
      "User-Agent": "HeadlessChrome/109",
    },
  });
  const text = await res.text();
  assertStringIncludes(text, "/es2021/");
});
