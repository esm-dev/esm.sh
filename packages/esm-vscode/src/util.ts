import { IText, parse, SyntaxKind, walk } from "html5parser";

export function getImportMapFromHtml(html: string) {
  let importMap = {};
  walk(parse(html), {
    enter: (node) => {
      if (
        node.type === SyntaxKind.Tag && node.name === "script" && node.body &&
        node.attributes.some((a) =>
          a.name.value === "type" && a.value?.value === "importmap"
        )
      ) {
        importMap = JSON.parse(
          node.body.map((a) => (a as IText).value).join(""),
        );
      }
    },
  });
  return importMap;
}
