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
        const { css: customCSS, entryPoints } = JSON.parse(source);
        const data = await Promise.all(entryPoints.map((url: string) => {
          return fetch(url).then((res) => res.text());
        }));
        const res = await (uno ??
          (uno = createGenerator({
            ...unoConfig,
            presets: (unoConfig.presets ?? []).concat(
              unocssPresets.map((f) => f(unoConfig)),
            ),
          })))
          .generate(data.join("\n"), {
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
        const entryPoints = atUse.map((entry) =>
          entry.slice(5).split(";")[0].split(",")
            .map((s) => s.trim()).filter(Boolean)
        ).flat()
          .map((s) => new URL(s, req.url));
        const checksums = await Promise.all(entryPoints.map((url) => {
          return fetch(url).then((res) => {
            const headers = res.headers;
            let etag = headers.get("etag");
            if (!etag) {
              const size = headers.get("content-length");
              const modtime = headers.get("last-modified");
              if (size && modtime) {
                etag = "W/" + size + "-" + modtime;
              }
            }
            if (etag) {
              res.body?.cancel();
              return etag;
            }
            return res.text();
          });
        }));
        const etag = await computeHash(
          new TextEncoder().encode(css + checksums.join("\n")),
        );
        return new Response(
          JSON.stringify({
            entryPoints,
            css: lines.join("\n"),
          }),
          { headers: { etag } },
        );
      },
      "eager",
    );
  },
};

/** compute the hash of the given input, default algorithm is SHA-1 */
async function computeHash(
  input: Uint8Array,
  algorithm: AlgorithmIdentifier = "SHA-1",
): Promise<string> {
  const buffer = new Uint8Array(await crypto.subtle.digest(algorithm, input));
  return [...buffer].map((b) => b.toString(16).padStart(2, "0")).join("");
}
