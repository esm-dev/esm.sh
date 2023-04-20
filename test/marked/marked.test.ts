import {
  assert,
  assertEquals,
} from "https://deno.land/std@0.180.0/testing/asserts.ts";

import marked from "http://localhost:8080/marked@2?deps=@types/marked@2.0.0";
import { safeLoadFront } from "http://localhost:8080/yaml-front-matter@4.1.1";

const md = `---
title: esm.sh
---

# esm.sh

A fast, global content delivery network to transform [NPM](http://npmjs.org/) packages to standard **ES Modules** by [esbuild](https://github.com/evanw/esbuild).
`;

Deno.test("marked(safeLoadFront)", () => {
  const { __content, ...meta } = safeLoadFront(md);
  const html = marked.parse(__content);
  assert(
    typeof html === "string" && html.includes('<h1 id="esmsh">esm.sh</h1>'),
  );
  assertEquals(meta, { title: "esm.sh" });
});
