import { assertEquals } from "jsr:@std/assert";

import sanitizeHtml, {
  defaults,
  simpleTransform,
} from "http://localhost:8080/sanitize-html@2.7.0?deps=@types/sanitize-html@2.6.2";
import type sanitize from "http://localhost:8080/@types/sanitize-html@2.6.2/index.d.ts";
import type { IOptions } from "http://localhost:8080/@types/sanitize-html@2.6.2/index.d.ts";
import type * as sanitizeTypes from "http://localhost:8080/@types/sanitize-html@2.6.2/index.d.ts";

Deno.test("sanitize-html default export and namespace types", () => {
  const options: IOptions = { allowedTags: ["b"] };
  const namespaceOptions: sanitize.IOptions = options;
  const namedOptions: sanitizeTypes.IOptions = namespaceOptions;
  const defaultOptions: sanitize.IDefaults = defaults;
  const transform: sanitize.Transformer = simpleTransform("b", {});
  const clean: string = sanitizeHtml(
    "<script>bad()</script><b>safe</b>",
    namedOptions,
  );
  assertEquals(clean, "<b>safe</b>");
  assertEquals(defaultOptions, sanitizeHtml.defaults);
  assertEquals(simpleTransform, sanitizeHtml.simpleTransform);
  assertEquals(transform("strong", {}).tagName, "b");
});
