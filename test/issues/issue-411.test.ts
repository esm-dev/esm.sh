import { assertStringIncludes } from "jsr:@std/assert";

import postcss from "http://localhost:8080/postcss@8.4.16";
import nested from "http://localhost:8080/postcss-nested@5.0.6?deps=postcss@8.4.16";

Deno.test("issue #411", () => {
	const { css } = postcss([nested]).process(`
.a {
	color: blue;

	& .b {
		color: red;
	}
}
`);
	assertStringIncludes(css, ".a .b {");
});
