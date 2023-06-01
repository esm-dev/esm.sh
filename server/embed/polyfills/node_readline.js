// https://nodejs.org/api/readline.html

function panic() {
  throw new Error(
    `[esm.sh] "node:readline" is not supported in browser environment.`,
  );
}

export class Interface {
  constructor() {
    panic();
  }
}

export function clearLine() {
  panic();
}

export function clearScreenDown() {
  panic();
}

export function createInterface() {
  return new Interface();
}

export function cursorTo() {
  panic();
}

export function emitKeypressEvents() {
  panic();
}

export function moveCursor() {
  panic();
}

export default {
  Interface,
  clearLine,
  clearScreenDown,
  createInterface,
  cursorTo,
  emitKeypressEvents,
  moveCursor,
};
