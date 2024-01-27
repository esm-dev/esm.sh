import * as monacoNS from "monaco-editor-core";
import * as ls from "../ls-types";
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
  const workerAccessor: ls.WorkerAccessor<HTMLWorker> = (
    ...uris: monacoNS.Uri[]
  ): Promise<HTMLWorker> => {
    return worker.withSyncedResources(uris);
  };

  ls.preclude(monaco);
  languages.registerCompletionItemProvider(
    languageId,
    new ls.CompletionAdapter(workerAccessor, [".", ":", "<", '"', "=", "/"]),
  );
  languages.registerHoverProvider(
    languageId,
    new ls.HoverAdapter(workerAccessor),
  );
  languages.registerDocumentHighlightProvider(
    languageId,
    new ls.DocumentHighlightAdapter(workerAccessor),
  );
  languages.registerLinkProvider(
    languageId,
    new ls.DocumentLinkAdapter(workerAccessor),
  );
  languages.registerFoldingRangeProvider(
    languageId,
    new ls.FoldingRangeAdapter(workerAccessor),
  );
  languages.registerDocumentSymbolProvider(
    languageId,
    new ls.DocumentSymbolAdapter(workerAccessor),
  );
  languages.registerSelectionRangeProvider(
    languageId,
    new ls.SelectionRangeAdapter(workerAccessor),
  );
  languages.registerRenameProvider(
    languageId,
    new ls.RenameAdapter(workerAccessor),
  );
  languages.registerDocumentFormattingEditProvider(
    languageId,
    new ls.DocumentFormattingEditProvider(workerAccessor),
  );
  languages.registerDocumentRangeFormattingEditProvider(
    languageId,
    new ls.DocumentRangeFormattingEditProvider(workerAccessor),
  );
}
