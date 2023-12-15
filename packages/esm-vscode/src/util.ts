import { IText, parse, SyntaxKind, walk } from "html5parser";
import type { ImportMap } from "./typescript-esmsh-plugin.ts";

export const regexpNpmNaming = /^[a-zA-Z0-9][\w\.\-]*$/;
export const regexpBuildVersion = /^(v\d+|stable)$/;
export const regexpSemVersion = /^v?\d+(\.\d+)*(-[\w\.]+)*$/;

export function isNEString(s: any): s is string {
  return typeof s === "string" && s.length > 0;
}

export function isValidEsmshUrl(
  v: string,
): { url: URL; name: string; version: string } | null {
  if (!v.startsWith("https://esm.sh/")) {
    return null;
  }
  let scope = "";
  let name = "";
  let version = "";
  const url = new URL(v);
  const parts = url.pathname.slice(1).split("/");
  if (regexpBuildVersion.test(parts[0])) {
    parts.shift();
  }
  name = parts.shift()!;
  console.log(name);
  if (name?.startsWith("@")) {
    scope = name;
    if (!regexpNpmNaming.test(scope.slice(1))) {
      return null;
    }
    name = parts.shift()!;
  }
  const idx = name.lastIndexOf("@");
  if (idx > 0) {
    version = name.slice(idx + 1);
    if (!regexpSemVersion.test(version)) {
      return null;
    }
    name = name.slice(0, idx);
  }
  if (!name || !regexpNpmNaming.test(name)) {
    return null;
  }
  return { url, name: scope ? scope + "/" + name : name, version };
}

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

export function debunce<T extends (...args: any[]) => any>(
  fn: T,
  timeout: number,
): T {
  let timer: number | undefined;
  return ((...args: any[]) => {
    if (timer) {
      clearTimeout(timer);
    }
    timer = setTimeout(() => {
      fn(...args);
    }, timeout);
  }) as any;
}
