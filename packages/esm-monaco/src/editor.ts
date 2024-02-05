import { editor, Uri } from "monaco-editor-core";

export const defaultEditorOptions: editor.IStandaloneEditorConstructionOptions =
  {
    automaticLayout: true,
    minimap: { enabled: false },
    scrollBeyondLastLine: false,
    theme: "vitesse-dark",
  };

const _create = editor.create;
const _createModel = editor.createModel;

// override default create and createModel methods
Object.assign(editor, {
  create: (
    container: HTMLElement,
    options?: editor.IStandaloneEditorConstructionOptions,
  ): editor.IStandaloneCodeEditor => {
    return _create(
      container,
      {
        ...defaultEditorOptions,
        ...options,
      } satisfies typeof options,
    );
  },
  createModel: (
    value: string,
    language?: string,
    uri?: string | URL | Uri,
  ) => {
    if (typeof uri === "string" || uri instanceof URL) {
      const url = new URL(uri, "file:///");
      uri = Uri.parse(url.href);
    }
    if (!language && uri) {
      // @ts-ignore getLanguageIdFromUri added by esm-monaco
      language = MonacoEnvironment.getLanguageIdFromUri?.(uri);
    }
    return _createModel(value, language, uri);
  },
});

export function workerUrl() {
  const m = workerUrl.toString().match(/import\(['"](.+?)['"]\)/);
  if (!m) throw new Error("worker url not found");
  const url = new URL(m[1], import.meta.url);
  Reflect.set(url, "import", () => import("./editor-worker.js")); // trick for bundlers
  return url;
}

export * from "monaco-editor-core";
