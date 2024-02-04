/*---------------------------------------------------------------------------------------------
 *  Copyright (c) Microsoft Corporation. All rights reserved.
 *  Licensed under the MIT License. See License.txt in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

import ts from "typescript";
import type * as monacoNS from "monaco-editor-core";
import { initialize } from "monaco-editor-core/esm/vs/editor/editor.worker";
import { type ImportMap, isBlank, resolve } from "../../import-map";
import { vfetch } from "../../vfs";

export interface Host {
  tryOpenModel(uri: string): Promise<boolean>;
  refreshDiagnostics: () => Promise<void>;
}

export interface ExtraLib {
  content: string;
  version: number;
}

export interface CreateData {
  compilerOptions: ts.CompilerOptions;
  libs: Record<string, string>;
  extraLibs: Record<string, ExtraLib>;
  importMap: ImportMap;
  inlayHintsOptions?: ts.UserPreferences;
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

export class TypeScriptWorker implements ts.LanguageServiceHost {
  private _ctx: monacoNS.worker.IWorkerContext<Host>;
  private _compilerOptions: ts.CompilerOptions;
  private _importMap: ImportMap;
  private _blankImportMap: boolean;
  private _importMapVersion: number;
  private _libs: Record<string, string>;
  private _extraLibs: Record<string, ExtraLib>;
  private _inlayHintsOptions?: ts.UserPreferences;
  private _languageService = ts.createLanguageService(this);
  private _httpLibs = new Map<string, string>();
  private _httpModules = new Map<string, string>();
  private _dtsMap = new Map<string, string>();
  private _badHttpRequests = new Set<string>();
  private _fetchPromises = new Map<string, Promise<void>>();

  constructor(
    ctx: monacoNS.worker.IWorkerContext<Host>,
    createData: CreateData,
  ) {
    this._ctx = ctx;
    this._compilerOptions = createData.compilerOptions;
    this._importMap = createData.importMap;
    this._blankImportMap = isBlank(createData.importMap);
    this._importMapVersion = 0;
    this._libs = createData.libs;
    this._extraLibs = createData.extraLibs;
    this._inlayHintsOptions = createData.inlayHintsOptions;
  }

  /*** language service host ***/

  getCompilationSettings(): ts.CompilerOptions {
    if (!this._compilerOptions.jsxImportSource) {
      const jsxImportSource = this._importMap.imports["@jsxImportSource"];
      if (jsxImportSource) {
        this._compilerOptions.jsxImportSource = jsxImportSource;
        if (!this._compilerOptions.jsx) {
          this._compilerOptions.jsx = ts.JsxEmit.ReactJSX;
        }
      }
    }
    return this._compilerOptions;
  }

  getLanguageService(): ts.LanguageService {
    return this._languageService;
  }

  getScriptFileNames(): string[] {
    return this._ctx.getMirrorModels()
      .map((model) => model.uri.toString())
      .concat(
        Object.keys(this._extraLibs),
        [...this._httpLibs.keys()],
        [...this._httpModules.keys()],
      );
  }

  getScriptVersion(fileName: string): string {
    if (fileName in this._extraLibs) {
      return String(this._extraLibs[fileName].version);
    }
    let model = this._getModel(fileName);
    if (model) {
      return model.version + "." + this._importMapVersion;
    }
    return "1"; // default lib is static
  }

  async getScriptText(fileName: string): Promise<string | undefined> {
    return this._getScriptText(fileName);
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
    if (
      fileName in this._libs || fileName in this._extraLibs ||
      this._httpLibs.has(fileName)
    ) {
      return ts.ScriptKind.TS;
    }
    if (this._httpModules.has(fileName)) {
      return ts.ScriptKind.JS;
    }
    const { pathname } = new URL(fileName, "file:///");
    const basename = pathname.substring(pathname.lastIndexOf("/") + 1);
    const dotIndex = basename.lastIndexOf(".");
    if (dotIndex === -1) {
      return ts.ScriptKind.JS;
    }
    const ext = basename.substring(dotIndex + 1);
    switch (ext) {
      case "ts":
        return ts.ScriptKind.TS;
      case "tsx":
        return ts.ScriptKind.TSX;
      case "js":
        return ts.ScriptKind.JS;
      case "jsx":
        return ts.ScriptKind.JSX;
      case "json":
        return ts.ScriptKind.JSON;
      default:
        return ts.ScriptKind.JS;
    }
  }

  getCurrentDirectory(): string {
    return "/";
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

  readFile(filename: string): string | undefined {
    return this._getScriptText(filename);
  }

  fileExists(filename: string): boolean {
    return this._fileExists(filename);
  }

  async getLibFiles(): Promise<Record<string, string>> {
    return this._libs;
  }

  resolveModuleNameLiterals(
    moduleLiterals: readonly ts.StringLiteralLike[],
    containingFile: string,
    redirectedReference: ts.ResolvedProjectReference | undefined,
    options: ts.CompilerOptions,
    containingSourceFile: ts.SourceFile,
    reusedNames: readonly ts.StringLiteralLike[] | undefined,
  ): readonly ts.ResolvedModuleWithFailedLookupLocations[] {
    return moduleLiterals.map((
      literal,
    ): ts.ResolvedModuleWithFailedLookupLocations["resolvedModule"] => {
      let moduleName = literal.text;
      if (!this._blankImportMap) {
        const resolved = resolve(this._importMap, moduleName, containingFile);
        moduleName = resolved.href;
      }
      if (TypeScriptWorker.getScriptExtension(moduleName, null) === null) {
        // use the extension of the containing file which is a dts file
        const ext = TypeScriptWorker.getScriptExtension(containingFile, null);
        if (ext === ".d.ts" || ext === ".d.mts" || ext === ".d.cts") {
          moduleName += ext;
        }
      }
      const moduleUrl = new URL(moduleName, containingFile);
      if (this._httpModules.has(containingFile)) {
        // ignore dependencies of http js modules
        return {
          resolvedFileName: moduleUrl.href,
          extension: ".js",
        };
      }
      if (moduleUrl.protocol === "file:") {
        const moduleHref = moduleUrl.href;
        for (const model of this._ctx.getMirrorModels()) {
          if (moduleHref === model.uri.toString()) {
            return {
              resolvedFileName: moduleHref,
              extension: TypeScriptWorker.getScriptExtension(moduleUrl),
            };
          }
        }
        this._ctx.host.tryOpenModel(moduleHref).then((ok) => {
          if (ok) {
            this._ctx.host.refreshDiagnostics();
          }
        });
      } else if (
        (
          moduleUrl.protocol === "http:" ||
          moduleUrl.protocol === "https:"
        ) &&
        moduleUrl.pathname !== "/" &&
        !/[@./-]$/.test(moduleUrl.pathname)
      ) {
        const moduleHref = moduleUrl.href;
        if (this._dtsMap.has(moduleHref)) {
          return {
            resolvedFileName: this._dtsMap.get(moduleHref),
            extension: ".d.ts",
          };
        }
        if (this._httpLibs.has(moduleHref)) {
          return {
            resolvedFileName: moduleHref,
            extension: ".d.ts",
          };
        }
        if (this._httpModules.has(moduleHref)) {
          return {
            resolvedFileName: moduleHref,
            extension: ".js",
          };
        }
        if (
          !this._fetchPromises.has(moduleHref) &&
          !this._badHttpRequests.has(moduleHref)
        ) {
          this._fetchPromises.set(
            moduleHref,
            vfetch(moduleUrl).then(async (res) => {
              if (res.ok) {
                const contentType = res.headers.get("content-type");
                const dts = res.headers.get("x-typescript-types");
                if (dts) {
                  const dtsRes = await vfetch(dts);
                  if (dtsRes.ok) {
                    res.body?.cancel();
                    this._httpLibs.set(dts, await dtsRes.text());
                    this._dtsMap.set(moduleHref, dts);
                  } else if (dtsRes.status >= 400 && dtsRes.status < 500) {
                    this._httpModules.set(moduleHref, await res.text());
                  } else {
                    res.body?.cancel();
                  }
                } else if (
                  /\.(c|m)?tsx?$/.test(moduleUrl.pathname) ||
                  /^(application|text)\/typescript/.test(contentType)
                ) {
                  this._httpLibs.set(moduleHref, await res.text());
                } else if (
                  /\.(c|m)?jsx?$/.test(moduleUrl.pathname) ||
                  /^(application|text)\/javascript/.test(contentType)
                ) {
                  this._httpModules.set(moduleHref, await res.text());
                } else {
                  // not a typescript or javascript file
                  res.body?.cancel();
                  this._badHttpRequests.add(moduleHref);
                }
              } else {
                res.body?.cancel();
                if (res.status >= 400 && res.status < 500) {
                  this._badHttpRequests.add(moduleHref);
                }
              }
              this._ctx.host.refreshDiagnostics();
            }).finally(() => {
              this._fetchPromises.delete(moduleHref);
            }),
          );
        }
      }
      return {
        resolvedFileName: moduleName,
        extension: TypeScriptWorker.getScriptExtension(moduleName),
      };
    }).map((resolvedModule) => ({ resolvedModule }));
  }

  /*** language features ***/

  async getSyntacticDiagnostics(fileName: string): Promise<Diagnostic[]> {
    const diagnostics = this._languageService.getSyntacticDiagnostics(fileName);
    return TypeScriptWorker.clearFiles(diagnostics);
  }

  async getSemanticDiagnostics(fileName: string): Promise<Diagnostic[]> {
    const diagnostics = this._languageService.getSemanticDiagnostics(fileName);
    return TypeScriptWorker.clearFiles(diagnostics);
  }

  async getSuggestionDiagnostics(fileName: string): Promise<Diagnostic[]> {
    const diagnostics = this._languageService.getSuggestionDiagnostics(
      fileName,
    );
    return TypeScriptWorker.clearFiles(diagnostics);
  }

  async getCompilerOptionsDiagnostics(
    fileName: string,
  ): Promise<Diagnostic[]> {
    const diagnostics = this._languageService.getCompilerOptionsDiagnostics();
    return TypeScriptWorker.clearFiles(diagnostics);
  }

  async getCompletionsAtPosition(
    fileName: string,
    position: number,
  ): Promise<ts.CompletionInfo | undefined> {
    const completions = this._languageService.getCompletionsAtPosition(
      fileName,
      position,
      {
        includeCompletionsForModuleExports: true,
        organizeImportsIgnoreCase: false,
        importModuleSpecifierPreference: "shortest",
        importModuleSpecifierEnding: "js",
        includePackageJsonAutoImports: "off",
        allowRenameOfImportPath: true,
      },
    );
    if (completions) {
      // filter auto-import suggestions from a types module
      completions.entries = completions.entries.filter(({ data }) =>
        !data || !TypeScriptWorker.isDts(data.fileName) ||
        !data.fileName.toLocaleLowerCase().startsWith(data.moduleSpecifier)
      );
    }
    return completions;
  }

  async getCompletionEntryDetails(
    fileName: string,
    position: number,
    entryName: string,
    data?: ts.CompletionEntryData,
  ): Promise<ts.CompletionEntryDetails | undefined> {
    try {
      return this._languageService.getCompletionEntryDetails(
        fileName,
        position,
        entryName,
        {
          insertSpaceAfterOpeningAndBeforeClosingNonemptyBrackets: true,
          semicolons: ts.SemicolonPreference.Insert,
        },
        undefined,
        { includeCompletionsForModuleExports: true },
        data,
      );
    } catch (error) {
      return;
    }
  }

  async getSignatureHelpItems(
    fileName: string,
    position: number,
    options: ts.SignatureHelpItemsOptions | undefined,
  ): Promise<ts.SignatureHelpItems | undefined> {
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
    const info = this._languageService.getQuickInfoAtPosition(
      fileName,
      position,
    );
    if (!info) {
      return;
    }

    // pettier display for module specifiers
    const { kind, kindModifiers, displayParts, textSpan } = info;
    if (
      kind === ts.ScriptElementKind.moduleElement &&
      displayParts?.length === 3
    ) {
      const moduleName = displayParts[2].text;
      if (
        // show full path for `file:` specifiers
        moduleName.startsWith('"file:') && fileName.startsWith("file:")
      ) {
        const model = this._getModel(fileName);
        const literalText = model.getValue().substring(
          textSpan.start,
          textSpan.start + textSpan.length,
        );
        const specifier = JSON.parse(literalText);
        info.displayParts[2].text = '"' +
          new URL(specifier, fileName).pathname + '"';
      } else if (
        // show module url for `http:` specifiers instead of the types url
        kindModifiers === "declare" && moduleName.startsWith('"http')
      ) {
        const specifier = JSON.parse(moduleName);
        for (const [url, dts] of this._dtsMap) {
          if (specifier + ".d.ts" === dts) {
            info.displayParts[2].text = '"' + url + '"';
            info.tags = [{
              name: "types",
              text: [{ kind: "text", text: dts }],
            }];
            if (url.startsWith("https://esm.sh/")) {
              const { pathname } = new URL(url);
              const pathSegments = pathname.split("/").slice(1);
              if (/^v\d$/.test(pathSegments[0])) {
                pathSegments.shift();
              }
              let scope = "";
              let pkgName = pathSegments.shift();
              if (pkgName?.startsWith("@")) {
                scope = pkgName;
                pkgName = pathSegments.shift();
              }
              if (!pkgName) {
                continue;
              }
              const npmPkgId = [scope, pkgName.split("@")[0]].filter(Boolean)
                .join("/");
              const npmPkgUrl = `https://www.npmjs.com/package/${npmPkgId}`;
              info.tags.unshift({
                name: "npm",
                text: [{ kind: "text", text: `[${npmPkgId}](${npmPkgUrl})` }],
              });
            }
            break;
          }
        }
      }
    }
    return info;
  }

  async getDocumentHighlights(
    fileName: string,
    position: number,
    filesToSearch: string[],
  ): Promise<ReadonlyArray<ts.DocumentHighlights> | undefined> {
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
    return this._languageService.getDefinitionAtPosition(fileName, position);
  }

  async getReferencesAtPosition(
    fileName: string,
    position: number,
  ): Promise<ts.ReferenceEntry[] | undefined> {
    return this._languageService.getReferencesAtPosition(fileName, position);
  }

  async getNavigationTree(
    fileName: string,
  ): Promise<ts.NavigationTree | undefined> {
    return this._languageService.getNavigationTree(fileName);
  }

  async getFormattingEditsForDocument(
    fileName: string,
    options: ts.FormatCodeSettings,
  ): Promise<ts.TextChange[]> {
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
    return this._languageService.getRenameInfo(fileName, position, options);
  }

  async getEmitOutput(fileName: string): Promise<ts.EmitOutput> {
    return this._languageService.getEmitOutput(fileName);
  }

  async getCodeFixesAtPosition(
    fileName: string,
    start: number,
    end: number,
    errorCodes: number[],
    formatOptions: ts.FormatCodeSettings,
  ): Promise<ReadonlyArray<ts.CodeFixAction>> {
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

  async provideInlayHints(
    fileName: string,
    start: number,
    end: number,
  ): Promise<readonly ts.InlayHint[]> {
    const preferences: ts.UserPreferences = this._inlayHintsOptions ?? {};
    const span: ts.TextSpan = { start, length: end - start };
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

  async organizeImports(
    fileName: string,
    formatOptions: ts.FormatCodeSettings,
  ): Promise<readonly ts.FileTextChanges[]> {
    try {
      return this._languageService.organizeImports(
        {
          type: "file",
          fileName,
          mode: ts.OrganizeImportsMode.SortAndCombine,
        },
        formatOptions,
        undefined,
      );
    } catch {
      return [];
    }
  }

  async updateCompilerOptions({
    compilerOptions,
    importMap,
    extraLibs,
  }: {
    compilerOptions?: ts.CompilerOptions;
    importMap?: ImportMap;
    extraLibs?: Record<string, ExtraLib>;
  }): Promise<void> {
    if (compilerOptions) {
      this._compilerOptions = compilerOptions;
    }
    if (importMap) {
      this._importMap = importMap;
      this._blankImportMap = isBlank(importMap);
      this._importMapVersion++;
    }
    if (extraLibs) {
      this._extraLibs = extraLibs;
    }
  }

  private static getScriptExtension(
    url: URL | string,
    defaultExt = ".js",
  ): string | null {
    const pathname = typeof url === "string"
      ? new URL(url, "file:///").pathname
      : url.pathname;
    const fileName = pathname.substring(pathname.lastIndexOf("/") + 1);
    const dotIndex = fileName.lastIndexOf(".");
    if (dotIndex === -1) {
      return defaultExt ?? null;
    }
    const ext = fileName.substring(dotIndex + 1);
    switch (ext) {
      case "ts":
        return fileName.endsWith(".d.ts") ? ".d.ts" : ".ts";
      case "mts":
        return fileName.endsWith(".d.mts") ? ".d.mts" : ".mts";
      case "cts":
        return fileName.endsWith(".d.cts") ? ".d.cts" : ".cts";
      case "tsx":
        return ".tsx";
      case "js":
        return ".js";
      case "mjs":
        return ".mjs";
      case "cjs":
        return ".cjs";
      case "jsx":
        return ".jsx";
      case "json":
        return ".json";
      default:
        return ".js";
    }
  }

  private static isDts(fileName: string): boolean {
    return fileName.endsWith(".d.ts") ||
      fileName.endsWith(".d.mts") ||
      fileName.endsWith(".d.cts");
  }

  private static clearFiles(tsDiagnostics: ts.Diagnostic[]): Diagnostic[] {
    // Clear the `file` field, which cannot be JSON stringified because it
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

  private _fileExists(fileName: string): boolean {
    let models = this._ctx.getMirrorModels();
    for (let i = 0; i < models.length; i++) {
      const uri = models[i].uri;
      if (uri.toString() === fileName || uri.toString(true) === fileName) {
        return true;
      }
    }
    return (
      fileName in this._libs ||
      `lib.${fileName}.d.ts` in this._libs ||
      fileName in this._extraLibs ||
      this._httpLibs.has(fileName) ||
      this._httpModules.has(fileName)
    );
  }

  private _getScriptText(fileName: string): string | undefined {
    let model = this._getModel(fileName);
    if (model) {
      return model.getValue();
    }
    return this._libs[fileName] ??
      this._libs[`lib.${fileName}.d.ts`] ??
      this._extraLibs[fileName]?.content ??
      this._httpLibs.get(fileName) ??
      this._httpModules.get(fileName);
  }

  private _getModel(fileName: string): monacoNS.worker.IMirrorModel | null {
    let models = this._ctx.getMirrorModels();
    for (let i = 0; i < models.length; i++) {
      const uri = models[i].uri;
      if (uri.toString() === fileName || uri.toString(true) === fileName) {
        return models[i];
      }
    }
    return null;
  }
}

globalThis.onmessage = () => {
  // ignore the first message
  initialize((ctx, createData) => {
    return new TypeScriptWorker(ctx, createData);
  });
};

export { ts as TS };
