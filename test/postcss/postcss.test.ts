import { assert } from "https://deno.land/std@0.155.0/testing/asserts.ts";

import postcss from "https://esm.sh/postcss@8.4.16";
import autoprefixer from "https://esm.sh/autoprefixer@10.4.12";

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
