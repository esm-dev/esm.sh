import { assertStringIncludes } from "https://deno.land/std@0.180.0/testing/asserts.ts";

import postcss from "http://localhost:8080/postcss@8.4.16";
import autoprefixer from "http://localhost:8080/autoprefixer@10.4.12?deps=postcss@8.4.16";
import tailwindCSS from "http://localhost:8080/tailwindcss@3.3.5?deps=postcss@8.4.16";

Deno.test("postcss(autoprefixer)", async () => {
  const { css } = await postcss([autoprefixer]).process(`
		backdrop-filter: blur(5px);
		user-select: none;
	`).async();
  assertStringIncludes(css, "-webkit-backdrop-filter: blur(5px);");
  assertStringIncludes(css, "-webkit-user-select: none;");
});

Deno.test("postcss(tailwindCSS)", async () => {
  const { css } = await postcss([
    tailwindCSS({
      content: [{ raw: '<div class="font-bold text-blue">', extension: "html" }],
      theme: {
        colors: {
          "blue": "rgba(31, 182, 255, 0.9)",
        },
      },
    }) as any,
  ]).process(`
    @tailwind base; /* Preflight will be injected here */
    @tailwind components;
    @tailwind utilities;
  `).async();

  assertStringIncludes(
    css,
    "1. Prevent padding and border from affecting element width.", // preflight.css
  );
  assertStringIncludes(css, ".font-bold {");
  assertStringIncludes(css, "font-weight: 700;");
  assertStringIncludes(css, ".text-blue {");
  assertStringIncludes(css, "color: rgba(31, 182, 255, 0.9);");
});
