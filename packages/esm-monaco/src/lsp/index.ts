import type * as monacoNS from "monaco-editor-core";

export interface LspLoader {
  alias?: string[];
  import: () => Promise<{
    setup: (languageId: string, monaco: typeof monacoNS) => Promise<void>;
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
    alias: ["javascript", "tsx"],
    import: () => import("./lsp/typescript/setup.js"),
  },
};
