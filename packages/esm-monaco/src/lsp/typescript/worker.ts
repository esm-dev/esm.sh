/*---------------------------------------------------------------------------------------------
 *  Copyright (c) Microsoft Corporation. All rights reserved.
 *  Licensed under the MIT License. See License.txt in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

import ts from "typescript";
import type * as monacoNS from "monaco-editor-core";
import * as worker from "monaco-editor-core/esm/vs/editor/editor.worker";
import type { ExtraLib, ImportMap } from "./api";

export interface CreateData {
  importMap: ImportMap;
  compilerOptions: ts.CompilerOptions;
  libs: Record<string, string>;
  extraLibs?: Record<string, ExtraLib>;
  inlayHintsOptions?: ts.UserPreferences;
}

export interface Host {
  tryOpenModel(uri: string): Promise<boolean>;
  refreshDiagnostics: () => Promise<void>;
}

/** TypeScriptWorker removes all but the `fileName` property to avoid serializing circular JSON structures. */
export interface DiagnosticRelatedInformation
  extends Omit<ts.DiagnosticRelatedInformation, "file"> {
  file: { fileName: string } | undefined;
}

/** May store more in future. For now, this will simply be `true` to indicate when a diagnostic is an unused-identifier diagnostic. */
export interface Diagnostic extends DiagnosticRelatedInformation {
  reportsUnnecessary?: {};
  reportsDeprecated?: {};
  source?: string;
  relatedInformation?: DiagnosticRelatedInformation[];
}

export interface DiagnosticsOptions {
  noSemanticValidation?: boolean;
  noSyntaxValidation?: boolean;
  noSuggestionDiagnostics?: boolean;
  /**
   * Limit diagnostic computation to only visible files.
   * Defaults to false.
   */
  onlyVisible?: boolean;
  diagnosticCodesToIgnore?: number[];
}

export interface DiagnosticsEvents {
  onDidChange: monacoNS.IEvent<void>;
  onDidExtraLibsChange: monacoNS.IEvent<void>;
}

export class TypeScriptWorker implements ts.LanguageServiceHost {
  private _ctx: monacoNS.worker.IWorkerContext<Host>;
  private _compilerOptions: ts.CompilerOptions;
  private _importMap: ImportMap;
  private _libs: Record<string, string>;
  private _extraLibs: Record<string, ExtraLib> = Object.create(null);
  private _inlayHintsOptions?: ts.UserPreferences;
  private _languageService = ts.createLanguageService(this);
  private _badModuleNames = new Set<string>();

  constructor(ctx: worker.IWorkerContext<Host>, createData: CreateData) {
    this._ctx = ctx;
    this._compilerOptions = createData.compilerOptions;
    this._libs = createData.libs;
    this._extraLibs = createData.extraLibs ?? {};
    this._inlayHintsOptions = createData.inlayHintsOptions;
  }

  resolveModuleNameLiterals(
    moduleLiterals: readonly ts.StringLiteralLike[],
    containingFile: string,
    redirectedReference: ts.ResolvedProjectReference | undefined,
    options: ts.CompilerOptions,
    containingSourceFile: ts.SourceFile,
    reusedNames: readonly ts.StringLiteralLike[] | undefined,
  ): readonly ts.ResolvedModuleWithFailedLookupLocations[] {
    return moduleLiterals.map((literal) => {
      if (
        literal.text.startsWith("file:///") || literal.text.startsWith("/") ||
        literal.text.startsWith(".")
      ) {
        const url = new URL(literal.text, containingFile);
        const isFileProtocol = url.protocol === "file:";
        if (isFileProtocol) {
          for (const model of this._ctx.getMirrorModels()) {
            if (url.href === model.uri.toString()) {
              return {
                resolvedModule: {
                  resolvedFileName: url.toString(),
                  extension: TypeScriptWorker.getFileExtension(url.pathname),
                },
              } satisfies ts.ResolvedModuleWithFailedLookupLocations;
            }
          }
        }
        if (isFileProtocol && !this._badModuleNames.has(url.href)) {
          this._ctx.host.tryOpenModel(url.href).then((ok) => {
            if (ok) {
              this._ctx.host.refreshDiagnostics();
            } else {
              // file not found, don't try to reopen it
              this._badModuleNames.add(url.href);
            }
          });
        }
      }
      return { resolvedModule: undefined };
    });
  }

  static getFileExtension(fileName: string): ts.Extension {
    const suffix = fileName.substring(fileName.lastIndexOf(".") + 1);
    switch (suffix) {
      case "ts":
        if (fileName.endsWith(".d.ts")) {
          return ts.Extension.Dts;
        }
        return ts.Extension.Ts;
      case "mts":
        if (fileName.endsWith(".d.mts")) {
          return ts.Extension.Dts;
        }
        return ts.Extension.Mts;
      case "tsx":
        return ts.Extension.Tsx;
      case "js":
        return ts.Extension.Js;
      case "mjs":
        return ts.Extension.Mjs;
      case "jsx":
        return ts.Extension.Jsx;
      case "json":
        return ts.Extension.Json;
      default:
        return ts.Extension.Ts;
    }
  }

  // --- language service host ---------------

  getCompilationSettings(): ts.CompilerOptions {
    return this._compilerOptions;
  }

  getLanguageService(): ts.LanguageService {
    return this._languageService;
  }

  getExtraLibs(): Record<string, ExtraLib> {
    return this._extraLibs;
  }

  getScriptFileNames(): string[] {
    const allModels = this._ctx.getMirrorModels().map((model) => model.uri);
    const models = allModels.filter((uri) => !this._fileNameIsLib(uri)).map((
      uri,
    ) => uri.toString());
    return models.concat(Object.keys(this._extraLibs));
  }

  private _getModel(fileName: string): worker.IMirrorModel | null {
    let models = this._ctx.getMirrorModels();
    for (let i = 0; i < models.length; i++) {
      const uri = models[i].uri;
      if (uri.toString() === fileName || uri.toString(true) === fileName) {
        return models[i];
      }
    }
    return null;
  }

  getScriptVersion(fileName: string): string {
    let model = this._getModel(fileName);
    if (model) {
      return model.version.toString();
    } else if (this.isDefaultLibFileName(fileName)) {
      // default lib is static
      return "1";
    } else if (fileName in this._extraLibs) {
      return String(this._extraLibs[fileName].version);
    }
    return "";
  }

  async getScriptText(fileName: string): Promise<string | undefined> {
    return this._getScriptText(fileName);
  }

  _getScriptText(fileName: string): string | undefined {
    let text: string;
    let model = this._getModel(fileName);
    const libizedFileName = "lib." + fileName + ".d.ts";
    if (model) {
      // a true editor model
      text = model.getValue();
    } else if (fileName in this._libs) {
      text = this._libs[fileName];
    } else if (libizedFileName in this._libs) {
      text = this._libs[libizedFileName];
    } else if (fileName in this._extraLibs) {
      // extra lib
      text = this._extraLibs[fileName].content;
    } else {
      return;
    }

    return text;
  }

  getScriptSnapshot(fileName: string): ts.IScriptSnapshot | undefined {
    const text = this._getScriptText(fileName);
    if (text === undefined) {
      return;
    }

    return <ts.IScriptSnapshot> {
      getText: (start, end) => text.substring(start, end),
      getLength: () => text.length,
      getChangeRange: () => undefined,
    };
  }

  getScriptKind(fileName: string): ts.ScriptKind {
    const suffix = fileName.substring(fileName.lastIndexOf(".") + 1);
    switch (suffix) {
      case "ts":
        return ts.ScriptKind.TS;
      case "tsx":
        return ts.ScriptKind.TSX;
      case "js":
        return ts.ScriptKind.JS;
      case "jsx":
        return ts.ScriptKind.JSX;
      default:
        return ts.ScriptKind.TS;
    }
  }

  getCurrentDirectory(): string {
    return "";
  }

  getDefaultLibFileName(options: ts.CompilerOptions): string {
    switch (options.target) {
      case 99 /* ESNext */:
        const esnext = "lib.esnext.full.d.ts";
        if (esnext in this._libs || esnext in this._extraLibs) return esnext;
      case 7 /* ES2020 */:
      case 6 /* ES2019 */:
      case 5 /* ES2018 */:
      case 4 /* ES2017 */:
      case 3 /* ES2016 */:
      case 2 /* ES2015 */:
      default:
        // Support a dynamic lookup for the ES20XX version based on the target
        // which is safe unless TC39 changes their numbering system
        const eslib = `lib.es${2013 + (options.target || 99)}.full.d.ts`;
        // Note: This also looks in _extraLibs, If you want
        // to add support for additional target options, you will need to
        // add the extra dts files to _extraLibs via the API.
        if (eslib in this._libs || eslib in this._extraLibs) {
          return eslib;
        }

        return "lib.es6.d.ts"; // We don't use lib.es2015.full.d.ts due to breaking change.
      case 1:
      case 0:
        return "lib.d.ts";
    }
  }

  isDefaultLibFileName(fileName: string): boolean {
    return fileName === this.getDefaultLibFileName(this._compilerOptions);
  }

  readFile(path: string): string | undefined {
    return this._getScriptText(path);
  }

  fileExists(path: string): boolean {
    return this._getScriptText(path) !== undefined;
  }

  async getLibFiles(): Promise<Record<string, string>> {
    return this._libs;
  }

  // --- language features

  private static clearFiles(tsDiagnostics: ts.Diagnostic[]): Diagnostic[] {
    // Clear the `file` field, which cannot be JSON'yfied because it
    // contains cyclic data structures, except for the `fileName`
    // property.
    // Do a deep clone so we don't mutate the ts.Diagnostic object (see https://github.com/microsoft/monaco-editor/issues/2392)
    const diagnostics: Diagnostic[] = [];
    for (const tsDiagnostic of tsDiagnostics) {
      const diagnostic: Diagnostic = {
        ...tsDiagnostic,
        file: tsDiagnostic.file
          ? { fileName: tsDiagnostic.file.fileName }
          : undefined,
      };
      if (tsDiagnostic.relatedInformation) {
        diagnostic.relatedInformation = [];
        for (const tsRelatedDiagnostic of tsDiagnostic.relatedInformation) {
          const relatedDiagnostic: DiagnosticRelatedInformation = {
            ...tsRelatedDiagnostic,
          };
          relatedDiagnostic.file = relatedDiagnostic.file
            ? { fileName: relatedDiagnostic.file.fileName }
            : undefined;
          diagnostic.relatedInformation.push(relatedDiagnostic);
        }
      }
      diagnostics.push(diagnostic);
    }
    return diagnostics;
  }

  async getSyntacticDiagnostics(fileName: string): Promise<Diagnostic[]> {
    if (this._fileNameIsLib(fileName)) {
      return [];
    }
    const diagnostics = this._languageService.getSyntacticDiagnostics(fileName);
    return TypeScriptWorker.clearFiles(diagnostics);
  }

  async getSemanticDiagnostics(fileName: string): Promise<Diagnostic[]> {
    if (this._fileNameIsLib(fileName)) {
      return [];
    }
    const diagnostics = this._languageService.getSemanticDiagnostics(fileName);
    return TypeScriptWorker.clearFiles(diagnostics);
  }

  async getSuggestionDiagnostics(fileName: string): Promise<Diagnostic[]> {
    if (this._fileNameIsLib(fileName)) {
      return [];
    }
    const diagnostics = this._languageService.getSuggestionDiagnostics(
      fileName,
    );
    return TypeScriptWorker.clearFiles(diagnostics);
  }

  async getCompilerOptionsDiagnostics(
    fileName: string,
  ): Promise<Diagnostic[]> {
    if (this._fileNameIsLib(fileName)) {
      return [];
    }
    const diagnostics = this._languageService.getCompilerOptionsDiagnostics();
    return TypeScriptWorker.clearFiles(diagnostics);
  }

  async getCompletionsAtPosition(
    fileName: string,
    position: number,
  ): Promise<ts.CompletionInfo | undefined> {
    if (this._fileNameIsLib(fileName)) {
      return undefined;
    }
    return this._languageService.getCompletionsAtPosition(
      fileName,
      position,
      undefined,
    );
  }

  async getCompletionEntryDetails(
    fileName: string,
    position: number,
    entry: string,
  ): Promise<ts.CompletionEntryDetails | undefined> {
    return this._languageService.getCompletionEntryDetails(
      fileName,
      position,
      entry,
      undefined,
      undefined,
      undefined,
      undefined,
    );
  }

  async getSignatureHelpItems(
    fileName: string,
    position: number,
    options: ts.SignatureHelpItemsOptions | undefined,
  ): Promise<ts.SignatureHelpItems | undefined> {
    if (this._fileNameIsLib(fileName)) {
      return undefined;
    }
    return this._languageService.getSignatureHelpItems(
      fileName,
      position,
      options,
    );
  }

  async getQuickInfoAtPosition(
    fileName: string,
    position: number,
  ): Promise<ts.QuickInfo | undefined> {
    if (this._fileNameIsLib(fileName)) {
      return undefined;
    }
    return this._languageService.getQuickInfoAtPosition(fileName, position);
  }

  async getDocumentHighlights(
    fileName: string,
    position: number,
    filesToSearch: string[],
  ): Promise<ReadonlyArray<ts.DocumentHighlights> | undefined> {
    if (this._fileNameIsLib(fileName)) {
      return undefined;
    }
    return this._languageService.getDocumentHighlights(
      fileName,
      position,
      filesToSearch,
    );
  }

  async getDefinitionAtPosition(
    fileName: string,
    position: number,
  ): Promise<ReadonlyArray<ts.DefinitionInfo> | undefined> {
    if (this._fileNameIsLib(fileName)) {
      return undefined;
    }
    return this._languageService.getDefinitionAtPosition(fileName, position);
  }

  async getReferencesAtPosition(
    fileName: string,
    position: number,
  ): Promise<ts.ReferenceEntry[] | undefined> {
    if (this._fileNameIsLib(fileName)) {
      return undefined;
    }
    return this._languageService.getReferencesAtPosition(fileName, position);
  }

  async getNavigationTree(
    fileName: string,
  ): Promise<ts.NavigationTree | undefined> {
    if (this._fileNameIsLib(fileName)) {
      return undefined;
    }
    return this._languageService.getNavigationTree(fileName);
  }

  async getFormattingEditsForDocument(
    fileName: string,
    options: ts.FormatCodeSettings,
  ): Promise<ts.TextChange[]> {
    if (this._fileNameIsLib(fileName)) {
      return [];
    }
    return this._languageService.getFormattingEditsForDocument(
      fileName,
      options,
    );
  }

  async getFormattingEditsForRange(
    fileName: string,
    start: number,
    end: number,
    options: ts.FormatCodeSettings,
  ): Promise<ts.TextChange[]> {
    if (this._fileNameIsLib(fileName)) {
      return [];
    }
    return this._languageService.getFormattingEditsForRange(
      fileName,
      start,
      end,
      options,
    );
  }

  async getFormattingEditsAfterKeystroke(
    fileName: string,
    postion: number,
    ch: string,
    options: ts.FormatCodeSettings,
  ): Promise<ts.TextChange[]> {
    if (this._fileNameIsLib(fileName)) {
      return [];
    }
    return this._languageService.getFormattingEditsAfterKeystroke(
      fileName,
      postion,
      ch,
      options,
    );
  }

  async findRenameLocations(
    fileName: string,
    position: number,
    findInStrings: boolean,
    findInComments: boolean,
    providePrefixAndSuffixTextForRename: boolean,
  ): Promise<readonly ts.RenameLocation[] | undefined> {
    if (this._fileNameIsLib(fileName)) {
      return undefined;
    }
    return this._languageService.findRenameLocations(
      fileName,
      position,
      findInStrings,
      findInComments,
      providePrefixAndSuffixTextForRename,
    );
  }

  async getRenameInfo(
    fileName: string,
    position: number,
    options: ts.UserPreferences,
  ): Promise<ts.RenameInfo> {
    if (this._fileNameIsLib(fileName)) {
      return {
        canRename: false,
        localizedErrorMessage: "Cannot rename in lib file",
      };
    }
    return this._languageService.getRenameInfo(fileName, position, options);
  }

  async getEmitOutput(fileName: string): Promise<ts.EmitOutput> {
    if (this._fileNameIsLib(fileName)) {
      return { outputFiles: [], emitSkipped: true };
    }
    return this._languageService.getEmitOutput(fileName);
  }

  async getCodeFixesAtPosition(
    fileName: string,
    start: number,
    end: number,
    errorCodes: number[],
    formatOptions: ts.FormatCodeSettings,
  ): Promise<ReadonlyArray<ts.CodeFixAction>> {
    if (this._fileNameIsLib(fileName)) {
      return [];
    }
    const preferences = {};
    try {
      return this._languageService.getCodeFixesAtPosition(
        fileName,
        start,
        end,
        errorCodes,
        formatOptions,
        preferences,
      );
    } catch {
      return [];
    }
  }

  async updateCompilerOptions(
    compilerOptions: ts.CompilerOptions,
    importMap: ImportMap,
  ): Promise<void> {
    this._compilerOptions = compilerOptions;
    this._importMap = importMap;
  }

  async updateExtraLibs(extraLibs: Record<string, ExtraLib>): Promise<void> {
    this._extraLibs = extraLibs;
  }

  async provideInlayHints(
    fileName: string,
    start: number,
    end: number,
  ): Promise<readonly ts.InlayHint[]> {
    if (this._fileNameIsLib(fileName)) {
      return [];
    }
    const preferences: ts.UserPreferences = this._inlayHintsOptions ?? {};
    const span: ts.TextSpan = {
      start,
      length: end - start,
    };

    try {
      return this._languageService.provideInlayHints(
        fileName,
        span,
        preferences,
      );
    } catch {
      return [];
    }
  }

  /**
   * Loading a default lib as a source file will mess up TS completely.
   * So our strategy is to hide such a text model from TS.
   * See https://github.com/microsoft/monaco-editor/issues/2182
   */
  _fileNameIsLib(resource: monacoNS.Uri | string): boolean {
    if (typeof resource === "string") {
      if (resource.startsWith("file:///")) {
        return resource.substring(8) in this._libs;
      }
      return false;
    }
    if (resource.path.startsWith("/lib.")) {
      return resource.path.slice(1) in this._libs;
    }
    return false;
  }
}

globalThis.onmessage = () => {
  // ignore the first message
  worker.initialize((ctx, createData) => {
    return new TypeScriptWorker(ctx, createData);
  });
};

// export TS for html embeded script
export { ts as TS };
