import { assertEquals } from "jsr:@std/assert";
import {
  convertSchemaToHtml,
  type Options,
  type Schema,
} from "esm.sh/@thebeyondgroup/shopify-rich-text-renderer@2.1.1?target=deno&dev";

Deno.test("shopify rich text types and runtime", () => {
  const schema: Schema = {
    type: "root",
    children: [{
      type: "paragraph",
      children: [{ type: "text", value: "Hello" }],
    }],
  };
  const options: Options = { scoped: false };
  const html: string = convertSchemaToHtml(schema, options);
  assertEquals(html, "<p>Hello</p>");

  if (false) {
    // @ts-expect-error A schema cannot be a number.
    convertSchemaToHtml(42);
    // @ts-expect-error The result is a string.
    const invalid: number = convertSchemaToHtml(schema);
  }
});
