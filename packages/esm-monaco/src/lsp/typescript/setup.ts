import type * as monacoNS from "monaco-editor-core";
import * as lf from "./language-features";
import type {
  CreateData,
  DiagnosticsOptions,
  TypeScriptWorker,
} from "./worker";
import type { SetupData } from "./api";

export async function setup(
  languageId: string,
  monaco: typeof monacoNS,
  data: SetupData,
) {
  const languages = monaco.languages;
  const isJavaScript = languageId === "javascript";
  const { libFiles, onExtraLibsChange } = data;

  // TODO: check vfs first
  libFiles.setLibs(
    await fetch(new URL("./libs.json", import.meta.url))
      .then((res) => res.text()).then(JSON.parse),
  );

  const createData: CreateData = {
    compilerOptions: {
      allowNonTsExtensions: true,
      target: 99, // ScriptTarget.Latest,
      allowJs: isJavaScript,
    },
    libs: libFiles.libs,
    extraLibs: libFiles.extraLibs,
  };

  console.log(createData);

  // should allow users to override diagnostics options?
  const diagnosticsOptions: DiagnosticsOptions = {
    noSemanticValidation: isJavaScript,
    noSyntaxValidation: false,
    onlyVisible: false,
  };

  const worker = monaco.editor.createWebWorker<TypeScriptWorker>({
    moduleId: "lsp/typescript/worker",
    label: languageId,
    createData,
  });
  const workerWithResources = (
    ...uris: monacoNS.Uri[]
  ): Promise<TypeScriptWorker> => {
    return worker.withSyncedResources(uris);
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
