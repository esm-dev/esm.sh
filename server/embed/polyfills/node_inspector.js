// https://nodejs.org/api/inspector.html

function panic() {
  throw new Error(
    `[esm.sh] "node:inspector" is not supported in browser environment.`,
  );
}

export class Session {
  constructor() {
    panic();
  }
}

export function close() {
  panic();
}

export function open() {
  panic();
}

export function url() {
  panic();
}

export function waitForDebugger() {
  panic();
}

export const console = window.console;

export default {
  close,
  console: window.console,
  open,
  url,
  waitForDebugger,
};
