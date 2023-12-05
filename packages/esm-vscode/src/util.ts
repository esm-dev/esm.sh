import { IText, parse, SyntaxKind, walk } from "html5parser";
import type { ImportMap } from "./typescript-esm-plugin.ts";

export function getImportMapFromHtml(html: string) {
  let importMap: ImportMap = {};
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

export function insertImportMap(html: string, importMap: ImportMap): string {
  let start = -1;
  let end = -1;
  let firstScriptTagStart = -1;
  let headTagEnd = -1;

  walk(parse(html), {
    enter: (node) => {
      if (
        firstScriptTagStart === -1 && node.type === SyntaxKind.Tag &&
        node.name === "script"
      ) {
        firstScriptTagStart = node.start;
      }
      if (
        headTagEnd === -1 && node.type === SyntaxKind.Tag &&
        node.name === "head"
      ) {
        headTagEnd = node.end;
      }
      if (
        node.type === SyntaxKind.Tag && node.name === "script" && node.body &&
        node.attributes.some((a) =>
          a.name.value === "type" && a.value?.value === "importmap"
        )
      ) {
        start = node.start;
        end = node.end;
      }
    },
  });

  const EOL = "\n";
  const ident = "  ";
  const im = JSON.stringify(importMap, undefined, 2).split("\n")
    .map((l) => ident + ident + l).join(EOL);
  const imScript =
    `<script type="importmap">${EOL}${im}${EOL}${ident}</script>`;

  if (start > 0 && end > 0) {
    return html.slice(0, start) + imScript + html.slice(end);
  }

  if (firstScriptTagStart > 0) {
    return html.slice(0, firstScriptTagStart) + imScript + EOL + ident +
      html.slice(firstScriptTagStart);
  }

  if (headTagEnd > 0) {
    let offset = 0;
    let i = headTagEnd;
    while (html[i] && html[i] !== "<") {
      offset++;
      i--;
    }
    return html.slice(0, headTagEnd - offset) + ident + imScript + EOL +
      html.slice(headTagEnd - offset);
  }

  return html;
}

export function sortByVersion(a: string, b: string) {
  const [aMain, aPR] = a.split("-");
  const [bMain, bPR] = b.split("-");
  const aParts = aMain.split(".");
  const bParts = bMain.split(".");
  for (let i = 0; i < Math.max(aParts.length, bParts.length); i++) {
    const aPart = parseInt(aParts[i]) || 0;
    const bPart = parseInt(bParts[i]) || 0;
    if (aPart !== bPart) {
      return bPart - aPart;
    }
  }
  if (aPR && bPR) {
    return bPR.localeCompare(aPR);
  }
  return 0;
}
