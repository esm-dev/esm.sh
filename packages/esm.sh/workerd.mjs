import { initSync } from "esm-compiler";
import wasm from "esm-compiler/esm_compiler_bg.wasm";
initSync(wasm);

export * from "./src/index.mjs";
