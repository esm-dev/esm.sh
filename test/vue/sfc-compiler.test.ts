import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("Vue SFC Transpiling", async () => {
  const { transform } = await import("http://localhost:8080/@esm.sh/vue-loader@1.0.3");
  const ret = await transform(
    "/src/App.tsx",
    `<script setup lang="ts">const msg = 'Hello World!';</script><template><h1>{{msg}}</h1></template><style>h1{font-size: 32px}</style>`,
    {
      devRuntime: "/@dvr",
      isDev: true,
      imports: { "@vue/compiler-sfc": import("http://localhost:8080/@vue/compiler-sfc@3.5.8") } as any,
    },
  );
  assertEquals(ret.lang, "ts");
  assertStringIncludes(ret.code, "const msg = 'Hello World!'");
  assertStringIncludes(ret.code, "$SFC_render");
  assertStringIncludes(ret.code, '"h1"');
  assertStringIncludes(ret.code, "import.meta.hot.accept");
  assertStringIncludes(ret.code, '"/@dvr"');
  assertStringIncludes(ret.code, "font-size: 32px");
});
