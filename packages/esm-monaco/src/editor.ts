import * as monaco from "monaco-editor-core";
import lspIndex from "./lsp/index";
import { allGrammars, allThemes, initShiki } from "./shiki";
import { VFS } from "./vfs";

let defaultTheme = "vitesse-dark";

interface InitOptions {
  vfs?: VFS;
  themes?: (string | { name: string })[];
  preloadGrammars?: string[];
  customGrammars?: { name: string }[];
}

export async function init(options: InitOptions = {}) {
  if (Reflect.has(globalThis, "MonacoEnvironment")) {
    // already initialized
    return;
  }

  Reflect.set(globalThis, "MonacoEnvironment", {
    getWorker: (_workerId: string, label: string) => {
      let filename = "./editor-worker.js";
      if (lspIndex[label]) {
        filename = `./lsp/${lspIndex[label].id}/worker.js`;
      }
      const url = new URL(filename, import.meta.url);
      if (url.hostname === "esm.sh") {
        return import(url.href + "?worker").then(({ default: workerFactory }) =>
          workerFactory()
        );
      }
      return new Worker(url, { type: "module" });
    },
  });

  let themes = (options.themes || [defaultTheme]).filter((v) =>
    (typeof v === "string" && allThemes.some((t) => t.name === v)) ||
    (typeof v === "object" && v !== null && v.name)
  );
  if (themes.length > 0) {
    const theme = themes[0];
    defaultTheme = typeof theme === "string" ? theme : theme.name;
  } else {
    themes = [defaultTheme];
  }

  const setupDataMap = Object.fromEntries(
    await Promise.all(
      allGrammars.filter((g) => !!lspIndex[g.name]?.api).map(
        async (g) => {
          const lang = g.name;
          const { init } = await import(
            new URL(`./lsp/${lspIndex[lang].id}/api.js`, import.meta.url).href
          );
          return [lang, init(monaco, lang)];
        },
      ),
    ),
  );

  if (options.vfs) {
    try {
      const list = await options.vfs.list();
      for (const path of list) {
        const ext = path.split(".").pop();
        const preloadGrammars = options.preloadGrammars ??
          (options.preloadGrammars = []);
        if (ext && !preloadGrammars.includes(ext)) {
          preloadGrammars.push(ext);
        }
      }
    } catch {
      // ignore
    }
    Reflect.set(monaco.editor, "vfs", options.vfs);
  }

  await initShiki(monaco, {
    ...options,
    themes,
    onLanguage: async (id: string) => {
      if (lspIndex[id]) {
        const { setup } = await import(
          new URL(`./lsp/${lspIndex[id].id}/setup.js`, import.meta.url)
            .href
        );
        setup(id, monaco, setupDataMap[id]);
      }
    },
  });

  customElements.define(
    "monaco-editor",
    class extends HTMLElement {
      async connectedCallback() {
        const opts = {};
        let file = "";

        for (const attr of this.attributes) {
          let value: any = attr.value;
          if (attr.name === "file") {
            file = value;
            continue;
          }
          if (value === "") {
            value = true;
          } else {
            try {
              value = JSON.parse(value);
            } catch {
              // ignore
            }
          }
          opts[
            attr.name.replace(/-[a-z]/g, (t) => t.slice(1).toUpperCase())
          ] = value;
        }
        this.style.display = "block";
        const editor = create(this, opts);
        if (file && options.vfs) {
          const url = new URL(file, "file:///");
          const model = createModel(
            await options.vfs.readTextFile(url),
            undefined,
            url,
            true,
          );
          editor.setModel(model);
        }
      }
    },
  );
}

const _create = monaco.editor.create;
const _createModel = monaco.editor.createModel;

export function create(
  container: HTMLElement,
  options?: monaco.editor.IStandaloneEditorConstructionOptions,
): monaco.editor.IStandaloneCodeEditor {
  return _create(
    container,
    {
      automaticLayout: true,
      minimap: { enabled: false },
      theme: defaultTheme,
      ...options,
    } satisfies typeof options,
  );
}

export function createModel(
  value: string,
  language?: string,
  uri?: string | URL | monaco.Uri,
  fromVFS?: boolean,
) {
  if (typeof uri === "string" || uri instanceof URL) {
    const url = new URL(uri, "file:///");
    uri = monaco.Uri.parse(url.href);
  }
  if (!language) {
    const lastDot = uri.path.lastIndexOf(".");
    if (lastDot > 0) {
      const ext = uri.path.slice(lastDot + 1);
      const lang = allGrammars.find((g) =>
        g.name === ext || g.aliases?.includes(ext)
      );
      if (lang) {
        language = lang.name;
      }
    }
  }
  const model: monaco.editor.ITextModel = _createModel(value, language, uri);
  const vfs = Reflect.get(monaco.editor, "vfs") as VFS | undefined;
  if (vfs && uri) {
    const path = uri.path;
    if (!fromVFS) {
      vfs.writeFile(path, value);
    }
    let syncTimer: number | null = null;
    model.onDidChangeContent((e) => {
      if (syncTimer) {
        clearTimeout(syncTimer);
      }
      syncTimer = setTimeout(() => {
        syncTimer = null;
        vfs.writeFile(path, model.getValue());
      }, 500);
    });
  }
  return model;
}

// override default create and createModel methods
Object.assign(monaco.editor, { create, createModel });

export * from "monaco-editor-core";
export * from "./vfs";
