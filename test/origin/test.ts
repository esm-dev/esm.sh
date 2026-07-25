import { assert, assertEquals, assertStringIncludes } from "jsr:@std/assert";

const poison =
  `https://attacker.invalid" } = options; globalThis.PWNED = true; const { z = "`;

Deno.test("landing page ignores X-Real-Origin", async () => {
  const poisoned = await fetch("http://localhost:8080/", {
    headers: {
      "User-Agent": "Mozilla/5.0",
      "X-Real-Origin": poison,
    },
  });
  const poisonedHtml = await poisoned.text();
  assertEquals(poisoned.status, 200);
  assertStringIncludes(poisonedHtml, "http://localhost:8080/PKG");
  assert(!poisonedHtml.includes("attacker.invalid"));
  assert(!poisonedHtml.includes("globalThis.PWNED"));

  const honestHtml = await fetch("http://localhost:8080/", {
    headers: {
      "User-Agent": "Mozilla/5.0",
    },
  }).then((res) => res.text());
  assertEquals(honestHtml, poisonedHtml);
});

Deno.test("WASM module wrapper ignores X-Real-Origin", async () => {
  const res = await fetch(
    "http://localhost:8080/esm-compiler@0.7.2/pkg/esm_compiler_bg.wasm?module&origin-regression",
    {
      headers: {
        "X-Real-Origin": poison,
      },
    },
  );
  const code = await res.text();
  assertEquals(res.status, 200);
  assertStringIncludes(
    code,
    `fetch("http://localhost:8080/esm-compiler@0.7.2/pkg/esm_compiler_bg.wasm")`,
  );
  assert(!code.includes("attacker.invalid"));
  assert(!code.includes("globalThis.PWNED"));
});

Deno.test("redirects ignore X-Real-Origin", async () => {
  const res = await fetch("http://localhost:8080/react@^18.3.1/package.json", {
    redirect: "manual",
    headers: {
      "X-Real-Origin": poison,
    },
  });
  const location = res.headers.get("location");
  res.body?.cancel();
  assertEquals(res.status, 302);
  assert(location);
  assert(location.startsWith("http://localhost:8080/react@18."));
  assert(!location.includes("attacker.invalid"));
});
