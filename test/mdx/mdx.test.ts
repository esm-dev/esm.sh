import { assertStringIncludes } from "https://deno.land/std@0.210.0/testing/asserts.ts";

import { compile } from "http://localhost:8080/@mdx-js/mdx@2.1.3";

const md = `
export const Thing = () => <>World!</>

# Hello, <Thing />

\`\`\`js
console.log("Hello, World!")
\`\`\`
`;

Deno.test("mdx", async () => {
  const output = await compile(md, {
    jsxImportSource: "https://esm.sh/react@18.2.0",
  });
  const code = output.toString();
  assertStringIncludes(
    code,
    `import {Fragment as _Fragment, jsx as _jsx, jsxs as _jsxs} from "https://esm.sh/react@18.2.0/jsx-runtime";`,
  );
  assertStringIncludes(code, `"h1"`);
  assertStringIncludes(code, `"code"`);
  assertStringIncludes(code, `"pre"`);
  assertStringIncludes(code, `className: "language-js"`);
});
