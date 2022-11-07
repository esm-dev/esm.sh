import { assert } from "https://deno.land/std@0.162.0/testing/asserts.ts";

import postcss from "http://localhost:8080/postcss@8.4.16";
import autoprefixer from "http://localhost:8080/autoprefixer@10.4.12?deps=postcss@8.4.16";

Deno.test("postcss(autoprefixer)", async () => {
  const { css } = await postcss([autoprefixer]).process(`
		backdrop-filter: blur(5px);
		user-select: none;
	`).async();
  assert(
    typeof css === "string" &&
      css.includes("-webkit-backdrop-filter: blur(5px);") &&
      css.includes("-webkit-user-select: none;"),
  );
});
