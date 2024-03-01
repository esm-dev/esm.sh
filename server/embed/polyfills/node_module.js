// https://nodejs.org/api/module.html

function panic() {
  throw new Error(
    `[esm.sh] "node:module" is not supported in browser environment.`,
  );
}

export const builtinModules = [];
export const createRequire = panic;
export const runMain = panic;
export const isBuiltin = panic;
export const register = panic;
export const syncBuiltinESMExports = panic;
export const findSourceMap = panic;
export const wrap = panic;

export class Module {
  constructor() {
    panic();
  }
}

export class SourceMap {
  constructor() {
    panic();
  }
}

export default {
  builtinModules,
  createRequire,
  findSourceMap,
  isBuiltin,
  Module,
  register,
  runMain,
  SourceMap,
  syncBuiltinESMExports,
  wrap,
};
