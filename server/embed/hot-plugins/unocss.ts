/** @version: 0.58.0 */

import type { Hot } from "../types/hot.d.ts";
import {
  createGenerator,
  type UnoGenerator,
} from "https://esm.sh/@unocss/core@0.58.0";
import presetWind from "https://esm.sh/@unocss/preset-wind@0.58.0?bundle";

export default {
  name: "unocss",
  setup(hot: Hot) {
    const unoConfig: UnoConfig = {};
    const unocssPresets = hot.unocssPresets ?? (hot.unocssPresets = []);
    hot.unocss = {
      config(config) {
        Object.assign(unoConfig, config);
      },
    };
    unocssPresets.push((config) => presetWind(config.presetWind));
    let uno: UnoGenerator;
    hot.onLoad(
      /(^|\/|\.)uno.css$/,
      async (_url: URL, source: string, _options: Record<string, any> = {}) => {
        const { customCSS, data, entryPoints } = JSON.parse(source);
        const res = await (uno ??
          (uno = createGenerator({
            ...unoConfig,
            presets: (unoConfig.presets ?? []).concat(
              unocssPresets.map((f) => f(unoConfig)),
            ),
          })))
          .generate(data, {
            preflights: true,
            minify: true,
          });
        const transform = hot.unocssTransformDirectives;
        return {
          code: res.css +
            (transform ? await transform(customCSS, uno) : customCSS),
          contentType: "text/css; charset=utf-8",
          deps: entryPoints.map((url: string) => new URL(url).pathname),
        };
      },
      async (req) => {
        const css = await fetch(req).then((res) => res.text());
        const lines: string[] = [];
        const atUse: string[] = [];
        css.split("\n").map((line) => {
          const trimmed = line.trimStart();
          if (trimmed.startsWith("@use ")) {
            atUse.push(trimmed);
          } else {
            if (trimmed.startsWith("@media ")) {
              line = trimmed.replace(/^@media\s+hot/, "@media all");
            }
            lines.push(line);
          }
        });
        const entryPoints = atUse.map((line) =>
          line.slice(5).split(";")[0].split(",")
            .map((s) => s.trim()).filter(Boolean)
        ).flat()
          .map((s) => new URL(s, req.url));
        const data = await Promise.all(
          entryPoints.map((url) => fetch(url).then((res) => res.text())),
        );
        return new Response(
          JSON.stringify({
            entryPoints,
            data: data.join("\n"),
            customCSS: lines.join("\n"),
          }),
        );
      },
      "eager",
    );
  },
};
