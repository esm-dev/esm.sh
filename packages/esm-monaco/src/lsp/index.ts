import type * as monacoNS from "monaco-editor-core";
import type { VFS } from "./vfs";

export interface LspLoader {
  aliases?: string[];
  import: () => Promise<{
    setup: (
      languageId: string,
      monaco: typeof monacoNS,
      vfs?: VFS,
    ) => Promise<void>;
    workerUrl: () => URL;
  }>;
}

export default <Record<string, LspLoader>> {
  html: {
    import: () => import("./lsp/html/setup.js"),
  },
  css: {
    import: () => import("./lsp/css/setup.js"),
  },
  json: {
    import: () => import("./lsp/json/setup.js"),
  },
  typescript: {
    aliases: ["javascript", "tsx"],
    import: () => import("./lsp/typescript/setup.js"),
  },
};
