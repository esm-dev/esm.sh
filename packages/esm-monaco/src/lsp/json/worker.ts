/*---------------------------------------------------------------------------------------------
 *  Copyright (c) Microsoft Corporation. All rights reserved.
 *  Licensed under the MIT License. See License.txt in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
import type * as monacoNS from "monaco-editor-core";
import * as worker from "monaco-editor-core/esm/vs/editor/editor.worker";
import * as jsonService from "vscode-json-languageservice";

export interface Options {
  /**
   * Configures the CSS data types known by the langauge service.
   */
  readonly settings?:
    & jsonService.LanguageSettings
    & jsonService.DocumentLanguageSettings;
  /**
   * Settings for the CSS formatter.
   */
  readonly format?: jsonService.FormattingOptions;
}

export interface CreateData {
  languageId: string;
  options: Options;
}

export class JSONWorker {
  private _ctx: monacoNS.worker.IWorkerContext;
  private _languageId: string;
  private _languageService: jsonService.LanguageService;
  private _languageSettings?:
    & jsonService.LanguageSettings
    & jsonService.DocumentLanguageSettings;

  constructor(ctx: monacoNS.worker.IWorkerContext, createData: CreateData) {
    this._ctx = ctx;
    this._languageSettings = createData.options.settings;
    this._languageId = createData.languageId;
    this._languageService = jsonService.getLanguageService({
      workspaceContext: {
        resolveRelativePath: (relativePath: string, resource: string) => {
          const url = new URL(relativePath, resource);
          return url.href;
        },
      },
      schemaRequestService: (url: string) =>
        fetch(url).then((response) => response.text()),
      clientCapabilities: jsonService.ClientCapabilities.LATEST,
    });
    this._languageService.configure(this._languageSettings);
  }

  async doValidation(uri: string): Promise<jsonService.Diagnostic[]> {
    let document = this._getTextDocument(uri);
    if (document) {
      let jsonDocument = this._languageService.parseJSONDocument(document);
      return this._languageService.doValidation(
        document,
        jsonDocument,
        this._languageSettings,
      );
    }
    return Promise.resolve([]);
  }
  async doComplete(
    uri: string,
    position: jsonService.Position,
  ): Promise<jsonService.CompletionList | null> {
    let document = this._getTextDocument(uri);
    if (!document) {
      return null;
    }
    let jsonDocument = this._languageService.parseJSONDocument(document);
    return this._languageService.doComplete(document, position, jsonDocument);
  }
  async doResolve(
    item: jsonService.CompletionItem,
  ): Promise<jsonService.CompletionItem> {
    return this._languageService.doResolve(item);
  }
  async doHover(
    uri: string,
    position: jsonService.Position,
  ): Promise<jsonService.Hover | null> {
    let document = this._getTextDocument(uri);
    if (!document) {
      return null;
    }
    let jsonDocument = this._languageService.parseJSONDocument(document);
    return this._languageService.doHover(document, position, jsonDocument);
  }
  async format(
    uri: string,
    range: jsonService.Range | null,
    options: jsonService.FormattingOptions,
  ): Promise<jsonService.TextEdit[]> {
    let document = this._getTextDocument(uri);
    if (!document) {
      return [];
    }
    let textEdits = this._languageService.format(
      document,
      range!, /* TODO */
      options,
    );
    return Promise.resolve(textEdits);
  }
  async resetSchema(uri: string): Promise<boolean> {
    return Promise.resolve(this._languageService.resetSchema(uri));
  }
  async findDocumentSymbols(
    uri: string,
  ): Promise<jsonService.DocumentSymbol[]> {
    let document = this._getTextDocument(uri);
    if (!document) {
      return [];
    }
    let jsonDocument = this._languageService.parseJSONDocument(document);
    let symbols = this._languageService.findDocumentSymbols2(
      document,
      jsonDocument,
    );
    return Promise.resolve(symbols);
  }
  async findDocumentColors(
    uri: string,
  ): Promise<jsonService.ColorInformation[]> {
    let document = this._getTextDocument(uri);
    if (!document) {
      return [];
    }
    let jsonDocument = this._languageService.parseJSONDocument(document);
    let colorSymbols = this._languageService.findDocumentColors(
      document,
      jsonDocument,
    );
    return Promise.resolve(colorSymbols);
  }
  async getColorPresentations(
    uri: string,
    color: jsonService.Color,
    range: jsonService.Range,
  ): Promise<jsonService.ColorPresentation[]> {
    let document = this._getTextDocument(uri);
    if (!document) {
      return [];
    }
    let jsonDocument = this._languageService.parseJSONDocument(document);
    let colorPresentations = this._languageService.getColorPresentations(
      document,
      jsonDocument,
      color,
      range,
    );
    return Promise.resolve(colorPresentations);
  }
  async getFoldingRanges(
    uri: string,
    context?: { rangeLimit?: number },
  ): Promise<jsonService.FoldingRange[]> {
    let document = this._getTextDocument(uri);
    if (!document) {
      return [];
    }
    let ranges = this._languageService.getFoldingRanges(document, context);
    return Promise.resolve(ranges);
  }
  async getSelectionRanges(
    uri: string,
    positions: jsonService.Position[],
  ): Promise<jsonService.SelectionRange[]> {
    let document = this._getTextDocument(uri);
    if (!document) {
      return [];
    }
    let jsonDocument = this._languageService.parseJSONDocument(document);
    let ranges = this._languageService.getSelectionRanges(
      document,
      positions,
      jsonDocument,
    );
    return Promise.resolve(ranges);
  }
  async parseJSONDocument(
    uri: string,
  ): Promise<jsonService.JSONDocument | null> {
    let document = this._getTextDocument(uri);
    if (!document) {
      return null;
    }
    let jsonDocument = this._languageService.parseJSONDocument(document);
    return Promise.resolve(jsonDocument);
  }
  async getMatchingSchemas(uri: string): Promise<jsonService.MatchingSchema[]> {
    let document = this._getTextDocument(uri);
    if (!document) {
      return [];
    }
    let jsonDocument = this._languageService.parseJSONDocument(document);
    return Promise.resolve(
      this._languageService.getMatchingSchemas(document, jsonDocument),
    );
  }
  private _getTextDocument(uri: string): jsonService.TextDocument | null {
    let models = this._ctx.getMirrorModels();
    for (let model of models) {
      if (model.uri.toString() === uri) {
        return jsonService.TextDocument.create(
          uri,
          this._languageId,
          model.version,
          model.getValue(),
        );
      }
    }
    return null;
  }
}

globalThis.onmessage = () => {
  // ignore the first message
  worker.initialize((ctx, createData) => {
    return new JSONWorker(ctx, createData);
  });
};
