import type * as monacoNS from "monaco-editor-core";
import * as lf from "../language-features";
import type { CreateData, HTMLWorker } from "./worker";

export function setup(languageId: string, monaco: typeof monacoNS) {
  const languages = monaco.languages;
  const createData: CreateData = {
    languageId,
    options: {
      data: {
        useDefaultDataProvider: true,
      },
      suggest: {},
      format: {
        tabSize: 4,
        insertSpaces: false,
        wrapLineLength: 120,
        unformatted:
          'default": "a, abbr, acronym, b, bdo, big, br, button, cite, code, dfn, em, i, img, input, kbd, label, map, object, q, samp, select, small, span, strong, sub, sup, textarea, tt, var',
        contentUnformatted: "pre",
        indentInnerHtml: false,
        preserveNewLines: true,
        indentHandlebars: false,
        endWithNewline: false,
        extraLiners: "head, body, /html",
        wrapAttributes: "auto",
      },
    },
  };
  const worker = monaco.editor.createWebWorker<HTMLWorker>({
    moduleId: "lsp/html/worker",
    label: languageId,
    createData,
  });
  const workerAccessor: lf.WorkerAccessor<HTMLWorker> = (
    ...uris: monacoNS.Uri[]
  ): Promise<HTMLWorker> => {
    return worker.withSyncedResources(uris);
  };

  lf.preclude(monaco);
  languages.registerCompletionItemProvider(
    languageId,
    new lf.CompletionAdapter(workerAccessor, [".", ":", "<", '"', "=", "/"]),
  );
  languages.registerHoverProvider(
    languageId,
    new lf.HoverAdapter(workerAccessor),
  );
  languages.registerDocumentHighlightProvider(
    languageId,
    new lf.DocumentHighlightAdapter(workerAccessor),
  );
  languages.registerLinkProvider(
    languageId,
    new lf.DocumentLinkAdapter(workerAccessor),
  );
  languages.registerFoldingRangeProvider(
    languageId,
    new lf.FoldingRangeAdapter(workerAccessor),
  );
  languages.registerDocumentSymbolProvider(
    languageId,
    new lf.DocumentSymbolAdapter(workerAccessor),
  );
  languages.registerSelectionRangeProvider(
    languageId,
    new lf.SelectionRangeAdapter(workerAccessor),
  );
  languages.registerRenameProvider(
    languageId,
    new lf.RenameAdapter(workerAccessor),
  );
  languages.registerDocumentFormattingEditProvider(
    languageId,
    new lf.DocumentFormattingEditProvider(workerAccessor),
  );
  languages.registerDocumentRangeFormattingEditProvider(
    languageId,
    new lf.DocumentRangeFormattingEditProvider(workerAccessor),
  );
}
