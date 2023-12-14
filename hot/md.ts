import type { Hot } from "../server/embed/types/hot.d.ts";
import { init as initWasm, parse } from "https://esm.sh/markdown-wasm-es@1.2.1";

export default {
  name: "md",
  setup(hot: Hot) {
    let waiting: Promise<any> | null = null;
    const init = async () => {
      if (waiting === null) {
        waiting = initWasm();
      }
      await waiting;
    };

    hot.onLoad(
      /\.(md|markdown)$/,
      async (_url: URL, source: string, _options: Record<string, any> = {}) => {
        await init();
        const html = parse(source);
        return {
          code: html,
          contentType: "text/html; charset=utf-8",
        };
      },
    );
  },
};
