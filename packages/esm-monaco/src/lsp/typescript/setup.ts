import type * as monacoNS from "monaco-editor-core";
import * as lf from "./language-features";
import type {
  CreateData,
  DiagnosticsOptions,
  TypeScriptWorker,
} from "./worker";
import type { SetupData } from "./api";

// javascript and typescript share the same worker
let worker:
  | monacoNS.editor.MonacoWebWorker<TypeScriptWorker>
  | Promise<monacoNS.editor.MonacoWebWorker<TypeScriptWorker>>
  | null = null;

export async function setup(
  languageId: string,
  monaco: typeof monacoNS,
  data: SetupData,
) {
  const languages = monaco.languages;
  const { libFiles, onExtraLibsChange } = data;

  if (!worker) {
    worker = (async () => {
      // TODO: check vfs first
      libFiles.setLibs(
        (await import(new URL("./libs.js", import.meta.url).href)).default,
      );
      const createData: CreateData = {
        compilerOptions: {
          allowJs: true,
          allowImportingTsExtensions: true,
          moduleResolution: 100, // ModuleResolutionKind.Bundler,
          target: 99, // ScriptTarget.Latest,
        },
        libs: libFiles.libs,
        extraLibs: libFiles.extraLibs,
      };
      return monaco.editor.createWebWorker<TypeScriptWorker>({
        moduleId: "lsp/typescript/worker",
        label: languageId,
        keepIdleModels: true,
        createData,
      });
    })();
  }

  if (worker instanceof Promise) {
    worker = await worker;
  }

  const workerWithResources = (
    ...uris: monacoNS.Uri[]
  ): Promise<TypeScriptWorker> => {
    return (worker as monacoNS.editor.MonacoWebWorker<TypeScriptWorker>)
      .withSyncedResources(uris);
  };

  const diagnosticsOptions: DiagnosticsOptions = {
    noSemanticValidation: languageId === "javascript",
    noSyntaxValidation: false,
    onlyVisible: false,
  };

  lf.preclude(monaco);
  languages.registerCompletionItemProvider(
    languageId,
    new lf.SuggestAdapter(workerWithResources),
  );
  languages.registerSignatureHelpProvider(
    languageId,
    new lf.SignatureHelpAdapter(workerWithResources),
  );
  languages.registerHoverProvider(
    languageId,
    new lf.QuickInfoAdapter(workerWithResources),
  ),
    languages.registerDocumentHighlightProvider(
      languageId,
      new lf.DocumentHighlightAdapter(workerWithResources),
    );
  languages.registerDefinitionProvider(
    languageId,
    new lf.DefinitionAdapter(libFiles, workerWithResources),
  );
  languages.registerReferenceProvider(
    languageId,
    new lf.ReferenceAdapter(libFiles, workerWithResources),
  );
  languages.registerDocumentSymbolProvider(
    languageId,
    new lf.OutlineAdapter(workerWithResources),
  );
  languages.registerRenameProvider(
    languageId,
    new lf.RenameAdapter(libFiles, workerWithResources),
  );
  languages.registerDocumentRangeFormattingEditProvider(
    languageId,
    new lf.FormatAdapter(workerWithResources),
  );
  languages.registerOnTypeFormattingEditProvider(
    languageId,
    new lf.FormatOnTypeAdapter(workerWithResources),
  );
  languages.registerCodeActionProvider(
    languageId,
    new lf.CodeActionAdaptor(workerWithResources),
  );
  languages.registerInlayHintsProvider(
    languageId,
    new lf.InlayHintsAdapter(workerWithResources),
  );
  new lf.DiagnosticsAdapter(
    libFiles,
    diagnosticsOptions,
    onExtraLibsChange,
    languageId,
    workerWithResources,
  );
}
