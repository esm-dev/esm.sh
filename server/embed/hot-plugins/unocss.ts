/** @version: 0.58.0 */

import type { Hot } from "../types/hot.d.ts";
import {
  createGenerator,
  type UnoGenerator,
  type UserConfig,
} from "https://esm.sh/@unocss/core@0.58.0";
import presetWind from "https://esm.sh/@unocss/preset-wind@0.58.0?bundle";

export default {
  name: "unocss",
  setup(hot: Hot) {
    const unoConfig: UserConfig = {
      presets: [presetWind()],
    };
    hot.unocss = {
      config(config: UserConfig) {
        Object.assign(unoConfig, config);
      },
    };
    let uno: UnoGenerator;
    hot.onLoad(
      /(^|\/|\.)uno.css$/,
      async (_url: URL, source: string, _options: Record<string, any> = {}) => {
        const { deps } = JSON.parse(source);
        const data = await Promise.all(deps.map((url: string) => {
          return fetch(url).then((res) => res.text());
        }));
        const { css } = await (uno ?? (uno = createGenerator(unoConfig)))
          .generate(data.join("\n"), {
            preflights: true,
            minify: true,
          });
        return {
          code: css,
          contentType: "text/css; charset=utf-8",
          deps: deps.map((url: string) => new URL(url).pathname),
        };
      },
      async (req) => {
        const css = await fetch(req).then((res) => res.text());
        const deps = css.split("\n")
          .filter((line) => line.startsWith("@include "))
          .map((entry) =>
            entry.slice(9).split(",").map((s) => s.trim()).filter(Boolean)
          ).flat()
          .map((s) => new URL(s, req.url));
        const checksums = await Promise.all(deps.map((url) => {
          return fetch(url).then((res) => {
            const headers = res.headers;
            let etag = headers.get("etag");
            if (!etag) {
              const size = headers.get("content-length");
              const modtime = headers.get("last-modified");
              if (size && modtime) {
                etag = "W/" + JSON.stringify(
                  parseInt(size).toString(36) + "-" +
                    (new Date(modtime).getTime() / 1000).toString(36),
                );
              }
            }
            if (etag) {
              res.body?.cancel();
              return etag;
            }
            return res.text();
          });
        }));
        return new Response(JSON.stringify({ deps }), {
          headers: {
            etag: await computeHash(
              new TextEncoder().encode(css + checksums.join("\n")),
            ),
          },
        });
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
