import type { editor } from "monaco-editor-core";
import type { HighlighterCore, ThemeInput } from "@shikijs/core";

const DEFAULT_WINDOWS_FONT_FAMILY = "Consolas, 'Courier New', monospace";
const DEFAULT_MAC_FONT_FAMILY = "Menlo, Monaco, 'Courier New', monospace";
const DEFAULT_LINUX_FONT_FAMILY = "'Droid Sans Mono', 'monospace', monospace";
const LINE_NUMBERS_COLOR = "rgba(222, 220, 213, 0.31)";
const MINIMUM_LINE_HEIGHT = 8;

export interface RenderOptions
  extends editor.IStandaloneEditorConstructionOptions {
  lang: string;
  code: string;
  theme?: string;
  userAgent?: string;
  fontMaxDigitWidth?: number;
}

export function render(
  highlighter: HighlighterCore,
  options: RenderOptions,
): string {
  const userAgent = options.userAgent ?? globalThis.navigator?.userAgent ?? "";
  const isMacintosh = userAgent.includes("Macintosh");
  const isLinux = userAgent.includes("Linux");
  const GOLDEN_LINE_HEIGHT_RATIO = isMacintosh ? 1.5 : 1.35;
  const EDITOR_FONT_DEFAULTS = {
    fontFamily: isMacintosh
      ? DEFAULT_MAC_FONT_FAMILY
      : isLinux
      ? DEFAULT_LINUX_FONT_FAMILY
      : DEFAULT_WINDOWS_FONT_FAMILY,
    fontWeight: "normal",
    fontSize: isMacintosh ? 12 : 14,
    lineHeight: 0,
    letterSpacing: 0,
  };
  const {
    lang,
    code,
    padding,
    fontFamily = EDITOR_FONT_DEFAULTS.fontFamily,
    fontWeight = EDITOR_FONT_DEFAULTS.fontWeight,
    fontSize = EDITOR_FONT_DEFAULTS.fontSize,
    lineHeight = 0,
    letterSpacing = EDITOR_FONT_DEFAULTS.letterSpacing,
    lineNumbersMinChars = 5,
    lineDecorationsWidth = 10,
    fontMaxDigitWidth,
  } = options;

  if (!fontMaxDigitWidth && !globalThis.document) {
    throw new Error(
      "`fontMaxDigitWidth` option is required in non-browser environment",
    );
  }

  let computedlineHeight = lineHeight || fontSize * GOLDEN_LINE_HEIGHT_RATIO;
  if (computedlineHeight < MINIMUM_LINE_HEIGHT) {
    computedlineHeight = computedlineHeight * fontSize;
  }

  const maxDigitWidth = fontMaxDigitWidth ?? getMaxDigitWidth(
    [fontWeight, fontSize + "px", fontFamily].join(" "),
  );

  const lines = countLines(code);
  const lineNumbers = Array.from(
    { length: lines },
    (_, i) => `<code>${i + 1}</code>`,
  );
  const lineNumbersWidth = Math.round(
    Math.max(lineNumbersMinChars, String(lines).length) * maxDigitWidth,
  );
  const decorationsWidth = Number(lineDecorationsWidth) + 16;

  const style = [
    "display:flex",
    "width:100%",
    "height:100%",
    "overflow-x:auto",
    "overflow-y:hidden",
    "margin:0",
    "padding:0",
    "-webkit-text-size-adjust:100%",
    "font-feature-settings: 'liga' 0, 'calt' 0",
    "font-variation-settings: normal",
    "'SF Mono',Monaco,Menlo,Consolas,'Ubuntu Mono','Liberation Mono','DejaVu Sans Mono','Courier New',monospace",
  ];
  if (padding?.top) {
    style.push(`padding-top:${padding.top}px`);
  }
  if (padding?.bottom) {
    style.push(`padding-bottom:${padding.bottom}px`);
  }

  const html = highlighter.codeToHtml(code, {
    lang,
    theme: options.theme ?? highlighter.getLoadedThemes()[0],
  });
  const styleIndex = html.indexOf('style="') + 7;
  const lineStyle = [
    "margin:0",
    "padding:0",
    `font-family:${fontFamily}`,
    `font-weight:${fontWeight}`,
    `font-size:${fontSize}px`,
    `line-height: ${computedlineHeight}px`,
    `letter-spacing: ${letterSpacing}px`,
  ];
  const lineNumbersStyle = [
    ...lineStyle,
    "display:flex",
    "flex-direction:column",
    "flex-shrink:0",
    "align-items:flex-end",
    "user-select:none",
    `color:${LINE_NUMBERS_COLOR}`,
    `width:${lineNumbersWidth}px;`,
  ];
  const shikiStyle = html.slice(styleIndex, html.indexOf('"', styleIndex));
  const finHtml = html.slice(0, styleIndex) + lineStyle.join(";") +
    html.slice(styleIndex);
  style.push(shikiStyle);
  return `<div style="${style.join(";")}">
<div style="${lineNumbersStyle.join(";")}">
${lineNumbers.join("")}
</div>
<div style="flex-shrink:0;width:${decorationsWidth}px"></div>
${finHtml}
</div>`;
}

// https://stackoverflow.com/questions/118241/calculate-text-width-with-javascript
function getMaxDigitWidth(font: string) {
  const canvas = document.createElement("canvas");
  const context = canvas.getContext("2d");
  const widths: number[] = [];
  context.font = font;
  for (let i = 0; i < 10; i++) {
    const metrics = context.measureText(i.toString());
    widths.push(metrics.width);
  }
  return Math.max(...widths);
}

function countLines(str: string) {
  let n = 1;
  for (let i = 0; i < str.length; i++) {
    if (str[i] === "\r" && str[i + 1] === "\n") i++;
    if (str[i] === "\n" || str[i] === "\r") n++;
  }
  return n;
}
