import * as monacoNS from "monaco-editor-core";
import * as ls from "../ls-types";
import type { CreateData, CSSWorker } from "./worker";

export function setup(languageId: string, monaco: typeof monacoNS) {
  const languages = monaco.languages;
  const bus = new monaco.Emitter<void>();
  const createData: CreateData = {
    languageId,
    options: {
      data: {
        useDefaultDataProvider: true,
      },
      format: {
        newlineBetweenSelectors: true,
        newlineBetweenRules: true,
        spaceAroundSelectorSeparator: false,
        braceStyle: "collapse",
        preserveNewLines: true,
      },
    },
  };
  const worker = monaco.editor.createWebWorker<CSSWorker>({
    moduleId: "lsp/css/worker",
    label: languageId,
    createData,
  });
  const workerAccessor: ls.WorkerAccessor<CSSWorker> = (
    ...uris: monacoNS.Uri[]
  ): Promise<CSSWorker> => {
    return worker.withSyncedResources(uris);
  };

  ls.preclude(monaco);
  languages.registerCompletionItemProvider(
    languageId,
    new ls.CompletionAdapter(workerAccessor, ["/", "-", ":"]),
  );
  languages.registerHoverProvider(
    languageId,
    new ls.HoverAdapter(workerAccessor),
  );
  languages.registerDocumentHighlightProvider(
    languageId,
    new ls.DocumentHighlightAdapter(workerAccessor),
  );
  languages.registerDefinitionProvider(
    languageId,
    new ls.DefinitionAdapter(workerAccessor),
  );
  languages.registerReferenceProvider(
    languageId,
    new ls.ReferenceAdapter(workerAccessor),
  );
  languages.registerDocumentSymbolProvider(
    languageId,
    new ls.DocumentSymbolAdapter(workerAccessor),
  );
  languages.registerRenameProvider(
    languageId,
    new ls.RenameAdapter(workerAccessor),
  );
  languages.registerColorProvider(
    languageId,
    new ls.DocumentColorAdapter(workerAccessor),
  );
  languages.registerFoldingRangeProvider(
    languageId,
    new ls.FoldingRangeAdapter(workerAccessor),
  );
  languages.registerSelectionRangeProvider(
    languageId,
    new ls.SelectionRangeAdapter(workerAccessor),
  );
  languages.registerDocumentFormattingEditProvider(
    languageId,
    new ls.DocumentFormattingEditProvider(workerAccessor),
  );
  languages.registerDocumentRangeFormattingEditProvider(
    languageId,
    new ls.DocumentRangeFormattingEditProvider(workerAccessor),
  );
  new ls.DiagnosticsAdapter(
    languageId,
    workerAccessor,
    bus.event,
  );
}
