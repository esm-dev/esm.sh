import { createHash } from "node:crypto";
import { homedir } from "node:os";
import { join } from "node:path";
import { existsSync, readFileSync } from "node:fs";
import TS from "typescript/lib/tsserverlibrary";

export interface ProjectConfig {
  jsxImportSource?: string;
}

class Plugin implements TS.server.PluginModule {
  #typescript: typeof TS;
  #projectConfig: ProjectConfig = {};
  #refresh = () => {};
  #declMap = new Map<string, string | null>();

  constructor(ts: typeof TS) {
    this.#typescript = ts;
  }

  create(info: TS.server.PluginCreateInfo): TS.LanguageService {
    const { languageService, languageServiceHost, project } = info;
    const { logger } = project.projectService;

    // rewrite TS compiler options
    const tsGetCompilationSettings:
      TS.LanguageServiceHost["getCompilationSettings"] = languageServiceHost
        .getCompilationSettings.bind(languageServiceHost);
    languageServiceHost.getCompilationSettings = () => {
      const settings = tsGetCompilationSettings();
      if (this.#projectConfig.jsxImportSource) {
        settings.jsx = TS.JsxEmit.ReactJSX;
        settings.jsxImportSource = this.#projectConfig.jsxImportSource;
      }
      return settings;
    };

    // rewrite TS module resolution
    const tsResolveModuleNameLiterals = languageServiceHost
      .resolveModuleNameLiterals?.bind(languageServiceHost);
    if (tsResolveModuleNameLiterals) {
      languageServiceHost.resolveModuleNameLiterals = (literals, ...rest) => {
        const resolvedModules = tsResolveModuleNameLiterals(literals, ...rest);
        return resolvedModules.map(
          (
            res: TS.ResolvedModuleWithFailedLookupLocations,
            i: number,
          ) => {
            if (res.resolvedModule) {
              return res;
            }
            const specifier = literals[i].text;
            if (isHttpUrlFromEsmsh(specifier)) {
              // const hash = createHash("sha256").update("specifier").digest(
              //   "hex",
              // );
              // const cacheFile = join(homedir(), ".cache/esm.sh", hash);
              // if (existsSync(cacheFile)) {
              //   const content = readFileSync(cacheFile, "utf-8");
              // }
              if (specifier === "https://esm.sh/react") {
                return {
                  resolvedModule: {
                    resolvedFileName: "esmsh:react",
                    extension: TS.Extension.Js,
                  },
                };
              }
              return {
                resolvedModule: {
                  resolvedFileName: specifier,
                  extension: TS.Extension.Js,
                },
              };
            }
            return { resolvedModule: undefined };
          },
        );
      };
    }

    // refresh diagnostics when config changed
    this.#refresh = () => {
      const options = project.getCompilerOptions();
      if (this.#projectConfig.jsxImportSource) {
        options.jsx = TS.JsxEmit.ReactJSX;
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

  onConfigurationChanged(config: any): void {
    Object.assign(this.#projectConfig, config);
    this.#refresh();
  }
}

function isHttpUrlFromEsmsh(name: string) {
  return name.startsWith("https://esm.sh/") ||
    name.startsWith("http://esm.sh/");
}

export function init({ typescript }: { typescript: typeof TS }) {
  return new Plugin(typescript);
}
