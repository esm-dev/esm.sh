export interface ImportMap {
  $src?: string;
  $support?: boolean;
  $baseURL: string;
  imports: Record<string, string>;
  scopes: Record<string, ImportMap["imports"]>;
}

export function blankImportMap(): ImportMap {
  return {
    $baseURL: "file:///",
    imports: {},
    scopes: {},
  };
}

export function isBlank(importMap: ImportMap) {
  return (
    Object.keys(importMap.imports).length === 0 &&
    Object.keys(importMap.scopes).length === 0
  );
}

function matchImports(specifier: string, imports: ImportMap["imports"]) {
  if (specifier in imports) {
    return imports[specifier];
  }
  for (const [k, v] of Object.entries(imports)) {
    if (k.endsWith("/") && specifier.startsWith(k)) {
      return v + specifier.slice(k.length);
    }
  }
  return null;
}

export function resolve(
  importMap: ImportMap,
  specifier: string,
  scriptUrlRaw: string,
) {
  const { $baseURL, imports, scopes } = importMap;
  const scriptUrl = new URL(scriptUrlRaw);
  const sameOriginScopes = Object.entries(scopes)
    .map(([scope, imports]) => [new URL(scope, $baseURL), imports] as const)
    .filter(([scopeUrl]) => scopeUrl.origin === scriptUrl.origin)
    .sort(([a], [b]) =>
      b.pathname.split("/").length - a.pathname.split("/").length
    );
  if (sameOriginScopes.length > 0) {
    for (const [scopeUrl, scopeImports] of sameOriginScopes) {
      if (scriptUrl.pathname.startsWith(scopeUrl.pathname)) {
        const match = matchImports(specifier, scopeImports);
        if (match) {
          return new URL(match, scopeUrl);
        }
      }
    }
  }
  const match = matchImports(specifier, imports);
  if (match) {
    return new URL(match, scriptUrl);
  }
  return new URL(specifier, scriptUrl);
}

export function parseImportMapFromJson(
  json: string,
  baseURL?: string,
): ImportMap {
  const importMap: ImportMap = {
    $support: globalThis.HTMLScriptElement?.supports?.("importmap"),
    $baseURL: new URL(baseURL ?? ".", "file:///").href,
    imports: {},
    scopes: {},
  };
  const v = JSON.parse(json);
  if (isObject(v)) {
    const { imports, scopes } = v;
    if (isObject(imports)) {
      validateImports(imports);
      importMap.imports = imports as ImportMap["imports"];
    }
    if (isObject(scopes)) {
      validateScopes(scopes);
      importMap.scopes = scopes as ImportMap["scopes"];
    }
  }
  return importMap;
}

function validateImports(imports: Record<string, unknown>) {
  for (const [k, v] of Object.entries(imports)) {
    if (!v || typeof v !== "string") {
      delete imports[k];
    }
  }
}

function validateScopes(imports: Record<string, unknown>) {
  for (const [k, v] of Object.entries(imports)) {
    if (isObject(v)) {
      validateImports(v);
    } else {
      delete imports[k];
    }
  }
}

function isObject(v: unknown): v is Record<string, unknown> {
  return v && typeof v === "object" && !Array.isArray(v);
}
