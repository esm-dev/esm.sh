var __create = Object.create;
var __defProp = Object.defineProperty;
var __getOwnPropDesc = Object.getOwnPropertyDescriptor;
var __getOwnPropNames = Object.getOwnPropertyNames;
var __getProtoOf = Object.getPrototypeOf;
var __hasOwnProp = Object.prototype.hasOwnProperty;
var __export = (target, all) => {
  for (var name in all)
    __defProp(target, name, { get: all[name], enumerable: true });
};
var __copyProps = (to, from, except, desc) => {
  if (from && typeof from === "object" || typeof from === "function") {
    for (let key of __getOwnPropNames(from))
      if (!__hasOwnProp.call(to, key) && key !== except)
        __defProp(to, key, { get: () => from[key], enumerable: !(desc = __getOwnPropDesc(from, key)) || desc.enumerable });
  }
  return to;
};
var __toESM = (mod, isNodeMode, target) => (target = mod != null ? __create(__getProtoOf(mod)) : {}, __copyProps(
  // If the importer is in node compatibility mode or this is not an ESM
  // file that has been converted to a CommonJS file using a Babel-
  // compatible transform (i.e. "__esModule" has not been set), then set
  // "default" to the CommonJS "module.exports" for node compatibility.
  isNodeMode || !mod || !mod.__esModule ? __defProp(target, "default", { value: mod, enumerable: true }) : target,
  mod
));
var __toCommonJS = (mod) => __copyProps(__defProp({}, "__esModule", { value: true }), mod);

// src/typescript-esm-plugin.ts
var typescript_esm_plugin_exports = {};
__export(typescript_esm_plugin_exports, {
  init: () => init
});
module.exports = __toCommonJS(typescript_esm_plugin_exports);
var import_tsserverlibrary = __toESM(require("typescript/lib/tsserverlibrary"));
var Plugin = class {
  #typescript;
  #projectConfig = {};
  #refresh = () => {
  };
  #declMap = /* @__PURE__ */ new Map();
  constructor(ts) {
    this.#typescript = ts;
  }
  create(info) {
    const { languageService, languageServiceHost, project } = info;
    const { logger } = project.projectService;
    const tsGetCompilationSettings = languageServiceHost.getCompilationSettings.bind(languageServiceHost);
    languageServiceHost.getCompilationSettings = () => {
      const settings = tsGetCompilationSettings();
      if (this.#projectConfig.jsxImportSource) {
        settings.jsx = import_tsserverlibrary.default.JsxEmit.ReactJSX;
        settings.jsxImportSource = this.#projectConfig.jsxImportSource;
      }
      return settings;
    };
    const tsResolveModuleNameLiterals = languageServiceHost.resolveModuleNameLiterals?.bind(languageServiceHost);
    if (tsResolveModuleNameLiterals) {
      languageServiceHost.resolveModuleNameLiterals = (literals, ...rest) => {
        const resolvedModules = tsResolveModuleNameLiterals(literals, ...rest);
        return resolvedModules.map(
          (res, i) => {
            if (res.resolvedModule) {
              return res;
            }
            const specifier = literals[i].text;
            if (isHttpUrlFromEsmsh(specifier)) {
              if (specifier === "https://esm.sh/react") {
                return {
                  resolvedModule: {
                    resolvedFileName: "esmsh:react",
                    extension: import_tsserverlibrary.default.Extension.Js
                  }
                };
              }
              return {
                resolvedModule: {
                  resolvedFileName: specifier,
                  extension: import_tsserverlibrary.default.Extension.Js
                }
              };
            }
            return { resolvedModule: void 0 };
          }
        );
      };
    }
    this.#refresh = () => {
      const options = project.getCompilerOptions();
      if (this.#projectConfig.jsxImportSource) {
        options.jsx = import_tsserverlibrary.default.JsxEmit.ReactJSX;
        options.jsxImportSource = this.#projectConfig.jsxImportSource;
      }
      project.setCompilerOptions(options);
      project.markAsDirty();
      project.refreshDiagnostics();
      project.updateGraph();
      languageService.getProgram()?.emit();
    };
    return languageService;
  }
  onConfigurationChanged(config) {
    Object.assign(this.#projectConfig, config);
    this.#refresh();
  }
};
function isHttpUrlFromEsmsh(name) {
  return name.startsWith("https://esm.sh/") || name.startsWith("http://esm.sh/");
}
function init({ typescript }) {
  return new Plugin(typescript);
}
// Annotate the CommonJS export names for ESM import in node:
0 && (module.exports = {
  init
});
