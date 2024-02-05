/*---------------------------------------------------------------------------------------------
 *  Copyright (c) Microsoft Corporation. All rights reserved.
 *  Copyright (c) X. <i@jex.me>
 *  Licensed under the MIT License. See License.txt in the project root for license information.
 *--------------------------------------------------------------------------------------------*/

import type * as monacoNS from "monaco-editor-core";
import * as worker from "monaco-editor-core/esm/vs/editor/editor.worker";
import * as cssService from "vscode-css-languageservice";

export interface CSSDataConfiguration {
  /**
   * Defines whether the standard CSS properties, at-directives, pseudoClasses and pseudoElements are shown.
   */
  useDefaultDataProvider?: boolean;
  /**
   * Provides a set of custom data providers.
   */
  dataProviders?: { [providerId: string]: cssService.CSSDataV1 };
}

export interface Options {
  /**
   * Configures the CSS data types known by the langauge service.
   */
  readonly data?: CSSDataConfiguration;
  /**
   * Settings for the CSS formatter.
   */
  readonly format?: cssService.CSSFormatConfiguration;
}

export interface CreateData {
  languageId: string;
  options: Options;
}

export class CSSWorker {
  private _ctx: worker.IWorkerContext;
  private _languageId: string;
  private _languageSettings: Options;
  private _languageService: cssService.LanguageService;

  constructor(
    ctx: monacoNS.worker.IWorkerContext,
    createData: CreateData,
  ) {
    this._ctx = ctx;
    this._languageId = createData.languageId;
    this._languageSettings = createData.options;
    const data = createData.options.data;
    const customDataProviders: cssService.ICSSDataProvider[] = [];
    if (data?.dataProviders) {
      for (const id in data.dataProviders) {
        customDataProviders.push(
          cssService.newCSSDataProvider(data.dataProviders[id]),
        );
      }
    }
    const lsOptions: cssService.LanguageServiceOptions = {
      useDefaultDataProvider: data?.useDefaultDataProvider,
      customDataProviders,
    };
    this._languageService = cssService.getCSSLanguageService(lsOptions);
  }

  // --- language service host ---------------

  async doValidation(uri: string): Promise<cssService.Diagnostic[]> {
    const document = this._getTextDocument(uri);
    if (document) {
      const stylesheet = this._languageService.parseStylesheet(document);
      const diagnostics = this._languageService.doValidation(
        document,
        stylesheet,
      );
      return Promise.resolve(diagnostics);
    }
    return Promise.resolve([]);
  }

  async doComplete(
    uri: string,
    position: cssService.Position,
  ): Promise<cssService.CompletionList | null> {
    const document = this._getTextDocument(uri);
    if (!document) {
      return null;
    }
    const stylesheet = this._languageService.parseStylesheet(document);
    const completions = this._languageService.doComplete(
      document,
      position,
      stylesheet,
    );
    return Promise.resolve(completions);
  }

  async doHover(
    uri: string,
    position: cssService.Position,
  ): Promise<cssService.Hover | null> {
    const document = this._getTextDocument(uri);
    if (!document) {
      return null;
    }
    const stylesheet = this._languageService.parseStylesheet(document);
    const hover = this._languageService.doHover(document, position, stylesheet);
    return Promise.resolve(hover);
  }

  async findDefinition(
    uri: string,
    position: cssService.Position,
  ): Promise<cssService.Location | null> {
    const document = this._getTextDocument(uri);
    if (!document) {
      return null;
    }
    const stylesheet = this._languageService.parseStylesheet(document);
    const definition = this._languageService.findDefinition(
      document,
      position,
      stylesheet,
    );
    return Promise.resolve(definition);
  }

  async findReferences(
    uri: string,
    position: cssService.Position,
  ): Promise<cssService.Location[]> {
    const document = this._getTextDocument(uri);
    if (!document) {
      return [];
    }
    const stylesheet = this._languageService.parseStylesheet(document);
    const references = this._languageService.findReferences(
      document,
      position,
      stylesheet,
    );
    return Promise.resolve(references);
  }

  async findDocumentHighlights(
    uri: string,
    position: cssService.Position,
  ): Promise<cssService.DocumentHighlight[]> {
    const document = this._getTextDocument(uri);
    if (!document) {
      return [];
    }
    const stylesheet = this._languageService.parseStylesheet(document);
    const highlights = this._languageService.findDocumentHighlights(
      document,
      position,
      stylesheet,
    );
    return Promise.resolve(highlights);
  }

  async findDocumentSymbols(
    uri: string,
  ): Promise<cssService.SymbolInformation[]> {
    const document = this._getTextDocument(uri);
    if (!document) {
      return [];
    }
    const stylesheet = this._languageService.parseStylesheet(document);
    const symbols = this._languageService.findDocumentSymbols(
      document,
      stylesheet,
    );
    return Promise.resolve(symbols);
  }

  async doCodeActions(
    uri: string,
    range: cssService.Range,
    context: cssService.CodeActionContext,
  ): Promise<cssService.Command[]> {
    const document = this._getTextDocument(uri);
    if (!document) {
      return [];
    }
    const stylesheet = this._languageService.parseStylesheet(document);
    const actions = this._languageService.doCodeActions(
      document,
      range,
      context,
      stylesheet,
    );
    return Promise.resolve(actions);
  }

  async findDocumentColors(
    uri: string,
  ): Promise<cssService.ColorInformation[]> {
    const document = this._getTextDocument(uri);
    if (!document) {
      return [];
    }
    const stylesheet = this._languageService.parseStylesheet(document);
    const colorSymbols = this._languageService.findDocumentColors(
      document,
      stylesheet,
    );
    return Promise.resolve(colorSymbols);
  }

  async getColorPresentations(
    uri: string,
    color: cssService.Color,
    range: cssService.Range,
  ): Promise<cssService.ColorPresentation[]> {
    const document = this._getTextDocument(uri);
    if (!document) {
      return [];
    }
    const stylesheet = this._languageService.parseStylesheet(document);
    const colorPresentations = this._languageService.getColorPresentations(
      document,
      stylesheet,
      color,
      range,
    );
    return Promise.resolve(colorPresentations);
  }

  async getFoldingRanges(
    uri: string,
    context?: { rangeLimit?: number },
  ): Promise<cssService.FoldingRange[]> {
    const document = this._getTextDocument(uri);
    if (!document) {
      return [];
    }
    const ranges = this._languageService.getFoldingRanges(document, context);
    return Promise.resolve(ranges);
  }

  async getSelectionRanges(
    uri: string,
    positions: cssService.Position[],
  ): Promise<cssService.SelectionRange[]> {
    const document = this._getTextDocument(uri);
    if (!document) {
      return [];
    }
    const stylesheet = this._languageService.parseStylesheet(document);
    const ranges = this._languageService.getSelectionRanges(
      document,
      positions,
      stylesheet,
    );
    return Promise.resolve(ranges);
  }

  async doRename(
    uri: string,
    position: cssService.Position,
    newName: string,
  ): Promise<cssService.WorkspaceEdit | null> {
    const document = this._getTextDocument(uri);
    if (!document) {
      return null;
    }
    const stylesheet = this._languageService.parseStylesheet(document);
    const renames = this._languageService.doRename(
      document,
      position,
      newName,
      stylesheet,
    );
    return Promise.resolve(renames);
  }

  async format(
    uri: string,
    range: cssService.Range | null,
    options: cssService.CSSFormatConfiguration,
  ): Promise<cssService.TextEdit[]> {
    const document = this._getTextDocument(uri);
    if (!document) {
      return [];
    }
    const settings = { ...this._languageSettings.format, ...options };
    const textEdits = this._languageService.format(
      document,
      range!, /* TODO */
      settings,
    );
    return Promise.resolve(textEdits);
  }

  private _getTextDocument(uri: string): cssService.TextDocument | null {
    const models = this._ctx.getMirrorModels();
    for (const model of models) {
      if (model.uri.toString() === uri) {
        return cssService.TextDocument.create(
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
    return new CSSWorker(ctx, createData);
  });
};
