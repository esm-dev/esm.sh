import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("`?external` query", async () => {
  {
    const res = await fetch("http://localhost:8080/react-dom@18.3.1?external=react");
    assertStringIncludes(await res.text(), '"/react-dom@18.3.1/X-ZXJlYWN0/denonext/react-dom.mjs"');
  }
  {
    const res = await fetch("http://localhost:8080/preact@10.23.2/hooks?external=preact");
    assertStringIncludes(await res.text(), '"/preact@10.23.2/X-ZXByZWFjdA/denonext/hooks.mjs"');
  }
  {
    const res = await fetch("http://localhost:8080/*preact@10.23.2/jsx-runtime");
    assertStringIncludes(await res.text(), '"/*preact@10.23.2/denonext/jsx-runtime.mjs"');
  }
  {
    const res = await fetch("http://localhost:8080/*preact@10.23.2/denonext/jsx-runtime.mjs");
    assertStringIncludes(await res.text(), 'from"preact"');
  }
  {
    const res = await fetch("http://localhost:8080/*preact@10.23.2/denonext/hooks.mjs");
    assertStringIncludes(await res.text(), 'from"preact"');
  }
  {
    // react-dom?external=react
    const res = await fetch("http://localhost:8080/react-dom@19.0.0/X-ZXJlYWN0/es2022/client.mjs");
    const code = await res.text();
    assertStringIncludes(code, 'from"react"');
    assertStringIncludes(code, 'from"./react-dom.mjs');
    assertStringIncludes(code, 'from"/scheduler@');
  }
  {
    // react-dom?external=react,react-dom
    const res = await fetch("http://localhost:8080/react-dom@19.0.0/X-ZXJlYWN0LHJlYWN0LWRvbQ/es2022/client.mjs");
    const code = await res.text();
    assertStringIncludes(code, 'from"react"');
    assertStringIncludes(code, 'from"react-dom"');
    assertStringIncludes(code, 'from"/scheduler@');
  }
  {
    // react-dom?external=react,react-dom,scheduler
    const res = await fetch("http://localhost:8080/react-dom@19.0.0/X-ZXJlYWN0LHJlYWN0LWRvbSxzY2hlZHVsZXI/es2022/client.mjs");
    const code = await res.text();
    assertStringIncludes(code, 'from"react"');
    assertStringIncludes(code, 'from"react-dom"');
    assertStringIncludes(code, 'from"scheduler"');
  }
  {
    const res = await fetch("http://localhost:8080/*react-dom@19.0.0/es2022/client.mjs");
    const code = await res.text();
    assertStringIncludes(code, 'from"react"');
    assertStringIncludes(code, 'from"react-dom"');
    assertStringIncludes(code, 'from"scheduler"');
  }
  {
    const res = await fetch("http://localhost:8080/react-dom@19.0.0/client?external=react,react-dom,scheduler");
    assertStringIncludes(await res.text(), '"/react-dom@19.0.0/X-ZXJlYWN0LHJlYWN0LWRvbSxzY2hlZHVsZXI/denonext/client.mjs"');
  }
  {
    const res = await fetch("http://localhost:8080/react-dom@19.0.0/client?external=react,scheduler,react-dom");
    assertStringIncludes(await res.text(), '"/react-dom@19.0.0/X-ZXJlYWN0LHJlYWN0LWRvbSxzY2hlZHVsZXI/denonext/client.mjs"');
  }
  {
    const res = await fetch("http://localhost:8080/react-dom@19.0.0/client?external=scheduler,react,react-dom");
    assertStringIncludes(await res.text(), '"/react-dom@19.0.0/X-ZXJlYWN0LHJlYWN0LWRvbSxzY2hlZHVsZXI/denonext/client.mjs"');
  }
  // https://github.com/esm-dev/esm.sh/issues/1101
  {
    const res = await fetch("http://localhost:8080/htm@3.1.1/preact?external=preact");
    assertStringIncludes(await res.text(), '"/htm@3.1.1/X-ZXByZWFjdA/denonext/preact.mjs"');
  }
});

Deno.test("types with `?external`", async () => {
  const res = await fetch("http://localhost:8080/swr@1.3.0/X-ZXJlYWN0/dist/use-swr.d.ts");
  assertEquals(res.status, 200);
  assertEquals(res.headers.get("Content-type"), "application/typescript; charset=utf-8");
  const ts = await res.text();
  assertStringIncludes(ts, '/// <reference types="react" />');
  assertStringIncludes(ts, 'import("react")');
});

Deno.test("external nodejs builtin modules", async () => {
  const res = await fetch("http://localhost:8080/cheerio@0.22.0/es2022/cheerio.mjs");
  assertEquals(res.status, 200);
  assertStringIncludes(await res.text(), ` from "/node/buffer.mjs"`);

  const res2 = await fetch("http://localhost:8080/cheerio@0.22.0?target=es2022&external=node:buffer");
  res2.body?.cancel();
  assertEquals(res2.status, 200);
  const res3 = await fetch("http://localhost:8080" + res2.headers.get("x-esm-path"));
  assertEquals(res3.status, 200);
  assertStringIncludes(await res3.text(), ` from "node:buffer"`);
});

Deno.test("external namespace packages", async () => {
  // Test that using @radix-ui as external marks all @radix-ui/* packages as external
  {
    const res = await fetch("http://localhost:8080/@radix-ui/react-dropdown-menu@2.0.5?external=@radix-ui");
    res.body?.cancel();
    assertEquals(res.status, 200);
    const modulePath = res.headers.get("x-esm-path");
    const res2 = await fetch("http://localhost:8080" + modulePath);
    const code = await res2.text();
    // All @radix-ui packages should be external
    assertStringIncludes(code, 'from"@radix-ui/react-compose-refs"');
    assertStringIncludes(code, 'from"@radix-ui/react-context"');
    assertStringIncludes(code, 'from"@radix-ui/react-id"');
    assertStringIncludes(code, 'from"@radix-ui/react-menu"');
    assertStringIncludes(code, 'from"@radix-ui/react-primitive"');
    assertStringIncludes(code, 'from"@radix-ui/react-use-controllable-state"');
  }
});
