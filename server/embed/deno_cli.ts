export type ImportMap = {
  readonly imports: Record<string, string>;
  readonly scopes: Record<string, Record<string, string>>;
};

export type Package = {
  readonly name: string;
  readonly version: string;
  readonly dependencies?: Record<string, string>;
  readonly peerDependencies?: Record<string, string>;
  readonly exports?: Record<string, unknown> | string;
};

const VERSION = "v{VERSION}";

async function add(args: string[], options: string[]) {
  const pkgs = await Promise.all(args.map(fetchPkgInfo));
  const importMap = await loadImportMap();

  await Promise.all(pkgs.map((pkg) => addPkgToImportMap(pkg, importMap)));
  await saveImportMap(importMap);
  console.log(
    `Added ${pkgs.length} packages to %cimport_map.json`,
    "color:blue",
  );
  if (pkgs.length > 0) {
    console.log(
      pkgs.map((pkg, index) => {
        const tab = index === pkgs.length - 1 ? "└─" : "├─";
        return `${tab} ${pkg.name}@${pkg.version}`;
      }).join("\n"),
    );
  }
}

async function remove(args: string[], options: string[]) {
  const importMap = await loadImportMap();
  const toRemove = args.filter((name) => name in importMap.imports);
  for (const name of toRemove) {
    Reflect.deleteProperty(importMap.imports, name);
    Reflect.deleteProperty(importMap.imports, name + "/");
    Reflect.deleteProperty(importMap.scopes, name);
    Reflect.deleteProperty(importMap.scopes, name + "/");
  }
  await saveImportMap(importMap);
  console.log(`Removed ${toRemove.length} packages`);
  if (toRemove.length > 0) {
    console.log(
      toRemove.map((name, index) => {
        const tab = index === toRemove.length - 1 ? "└─" : "├─";
        return `${tab} ${name}`;
      }).join("\n"),
    );
  }
}

async function upgrade(args: string[], options: string[]) {
  const importMap = await loadImportMap();
  const lastest = options.includes("--latest");
  const toUpgrade =
    (args.length === 0
      ? Object.keys(importMap.imports).filter((name) =>
        importMap.imports[name].startsWith("https://esm.sh/")
      ).map((name) => {
        let version: string;
        if (lastest) {
          version = "latest";
        } else {
          const url = importMap.imports[name];
          const [, v] = url.match(/@(\d+\.\d+\.\d+(-[a-z0-9\-\.]+)?)/)!;
          if (!v.includes("-")) {
            version = v.split(".").slice(0, 2).join(".");
          } else {
            version = v;
          }
        }
        return `${name}@${version}`;
      })
      : args.filter((name) =>
        name in importMap.imports ||
        name.slice(0, name.lastIndexOf("@")) in importMap.imports
      ).map((name) => {
        let version: string;
        if ((name.startsWith("@") ? name.slice(1) : name).includes("@")) {
          const a = name.split("@");
          version = a.pop()!;
          name = a.join("@");
        } else if (lastest) {
          version = "latest";
        } else {
          const url = importMap.imports[name] ??
            importMap.imports[name.slice(0, name.lastIndexOf("@"))];
          const [, v] = url.match(/@(\d+\.\d+\.\d+(-[a-z0-9\-\.]+)?)/)!;
          if (!v.includes("-")) {
            version = v.split(".").slice(0, 2).join(".");
          } else {
            version = v;
          }
        }
        return `${name}@${version}`;
      }));
  const pkgs = await Promise.all(toUpgrade.map(fetchPkgInfo));
  const upgraded: Package[] = [];

  for (const pkg of pkgs) {
    if (await addPkgToImportMap(pkg, importMap)) {
      upgraded.push(pkg);
    }
  }

  await saveImportMap(importMap);
  console.log(`Upgraded ${upgraded.length} packages`);
  if (upgraded.length > 0) {
    console.log(
      upgraded.map((pkg, index) => {
        const tab = index === pkgs.length - 1 ? "└─" : "├─";
        return `${tab} ${pkg.name}@${pkg.version}`;
      }).join("\n"),
    );
  }
}

const cache = new Map<string, Package>();
async function fetchPkgInfo(name: string): Promise<Package> {
  if (cache.has(name)) {
    return Promise.resolve(cache.get(name)!);
  }

  const res = await fetch(`https://esm.sh/${name}/package.json`);
  if (res.status === 404) {
    console.error(`%cerror%c: Package "${name}" not found`, "color:red", "");
    Deno.exit(1);
  }
  if (!res.ok) {
    throw new Error(
      `Failed to fetch "${name}": ${res.status} ${res.statusText}`,
    );
  }

  const pkg = await res.json();
  if (!pkg.name || !pkg.version) {
    throw new Error(`Invalid package.json of "${name}"`);
  }

  cache.set(name, pkg);
  return pkg;
}

async function loadImportMap(): Promise<ImportMap> {
  const importMap: ImportMap = { imports: {}, scopes: {} };
  try {
    const raw = (await Deno.readTextFile("import_map.json")).trim();
    if (raw.startsWith("{") && raw.endsWith("}")) {
      const { imports, scopes } = JSON.parse(raw);
      if (imports) {
        Object.assign(importMap.imports, imports);
      }
      if (scopes) {
        Object.assign(importMap.scopes, scopes);
      }
    }
  } catch (err) {
    if (!(err instanceof Deno.errors.NotFound)) {
      throw err;
    }
  }
  return importMap;
}

async function saveImportMap(importMap: ImportMap): Promise<void> {
  // clean up
  for (const importName in importMap.imports) {
    for (const [scopeName, scope] of Object.entries(importMap.scopes)) {
      if (importName in scope) {
        Reflect.deleteProperty(scope, importName);
        if (Object.keys(scope).length === 0) {
          Reflect.deleteProperty(importMap.scopes, scopeName);
        }
      }
    }
  }

  // sort
  const sortedImports = sortImports(importMap.imports);
  const sortedScopes = Object.fromEntries(
    Object.entries(importMap.scopes).sort(sortByKey).map((
      [key, scope],
    ) => [key, sortImports(scope)]),
  );

  // write
  await Deno.writeTextFile(
    "import_map.json",
    JSON.stringify({ imports: sortedImports, scopes: sortedScopes }, null, 2),
  );
}

async function addPkgToImportMap(
  pkg: Package,
  importMap: ImportMap,
): Promise<boolean> {
  const [pkgUrl, hasSubModules] = getPkgUrl(pkg);
  if (importMap.imports[pkg.name] === pkgUrl) {
    return false;
  }
  importMap.imports[pkg.name] = pkgUrl;
  if (hasSubModules) {
    importMap.imports[pkg.name + "/"] = pkgUrl + "/";
  } else {
    Reflect.deleteProperty(importMap.imports, pkg.name + "/");
  }
  if (pkg.dependencies) {
    importMap.scopes[pkg.name] = {};
    if (hasSubModules) {
      importMap.scopes[pkg.name + "/"] = {};
    }
    for (const [depName, depVersion] of Object.entries(pkg.dependencies)) {
      const dep = `${depName}@${depVersion}`;
      const depPkg = await fetchPkgInfo(dep);
      const depUrl =
        `https://esm.sh/${VERSION}/${depPkg.name}@${depPkg.version}`;
      importMap.scopes[pkg.name][depName] = depUrl;
      if (hasSubModules) {
        importMap.scopes[pkg.name + "/"][depName] = depUrl;
      }
    }
  }
  return true;
}

function getPkgUrl(pkg: Package): [string, boolean] {
  const { name, version, exports, dependencies, peerDependencies } = pkg;
  const hasSubModules = typeof exports === "object" &&
    Object.keys(exports).some((key) => key.length >= 3 && key.startsWith("./"));

  if (
    (dependencies && Object.keys(dependencies).length > 0) ||
    (peerDependencies && Object.keys(peerDependencies).length > 0)
  ) {
    return [
      `https://esm.sh/${VERSION}/*${name}@${version}`,
      hasSubModules,
    ];
  }
  return [`https://esm.sh/${VERSION}/${name}@${version}`, hasSubModules];
}

function sortImports(imports: Record<string, string>) {
  return Object.fromEntries(
    Object.entries(imports).sort(sortByKey),
  );
}

function sortByKey(a: [string, unknown], b: [string, unknown]) {
  const [aName] = a;
  const [bName] = b;
  if (aName < bName) {
    return -1;
  }
  if (aName > bName) {
    return 1;
  }
  return 0;
}

if (import.meta.main) {
  const start = performance.now();
  const [command, ...args] = Deno.args;
  const commands = {
    add,
    remove,
    upgrade,
  };

  if (command === undefined || !(command in commands)) {
    console.error(`Command "${command}" not found`);
    Deno.exit(1);
  }

  try {
    await commands[command as keyof typeof commands](
      args.filter((arg) => !arg.startsWith("-")),
      args.filter((arg) => arg.startsWith("-")),
    );
    console.log(`✨ Done in ${(performance.now() - start).toFixed(2)}ms`);
  } catch (error) {
    throw error;
  }
}
