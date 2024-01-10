/** @version: 0.58.0 */

import type { Hot } from "../server/embed/types/hot.d.ts";
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
    unocssPresets.push((config) => presetWind(config.presetWind));
    Reflect.set(hot, "unocss", {
      config(config) {
        Object.assign(unoConfig, config);
      },
    });
    let uno: UnoGenerator;
    hot.onLoad(
      /(^|\/|\.)uno.css$/,
      async (_url: URL, source: string, _options: Record<string, any> = {}) => {
        const { css, data, entryPoints } = JSON.parse(source);
        const res = await (uno ?? (uno = createGenerator({
          ...unoConfig,
          presets: (unoConfig.presets ?? []).concat(
            unocssPresets.map((f) => f(unoConfig)),
          ),
        }))).generate(data, {
          preflights: true,
          minify: true,
        });
        const transform = hot.unocssTransformDirectives;
        return {
          code: res.css + (transform ? await transform(css, uno) : css),
          contentType: "text/css; charset=utf-8",
          deps: entryPoints.map((name: string) => "/" + name),
        };
      },
      // custom fetcher
      async (req) => {
        const res = await fetch(req);
        if (!res.ok) {
          return res;
        }

        const text = await res.text();
        const lines: string[] = [];
        const atUse: string[] = [];
        text.split("\n").map((line) => {
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

        const globUrl = new URL(hot.basePath + "@hot-glob", req.url);
        const globRes = await fetch(globUrl, {
          method: "POST",
          body: JSON.stringify({
            pattern: atUse.map((line) =>
              line.slice(5).replace(/;+\s*$/, "").replace(/^['"]|['"]$/g, "")
            ).join(),
          }),
        });
        if (!globRes.ok) {
          return globRes;
        }
        if (globRes.headers.get("content-type") !== "binary/glob") {
          return new Response("Unsppported /@hot-glob api", { status: 500 });
        }
        const data = await globRes.text();
        const n = parseInt(
          globRes.headers.get("x-glob-index")!.split(",", 1)[0],
        );
        const entryPoints = JSON.parse(data.slice(0, n));
        return new Response(
          JSON.stringify({
            entryPoints,
            data,
            css: lines.join("\n"),
          }),
        );
      },
      "eager",
    );
  },
};
