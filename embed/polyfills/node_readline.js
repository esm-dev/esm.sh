// https://nodejs.org/api/readline.html

function notImplemented(name) {
  throw new Error(`[esm.sh] readline: '${name}' is not implemented`)
}

export class Interface {
  constructor() {
    notImplemented('Interface')
  }
}

export function clearLine() {
  notImplemented('clearLine')
}

export function clearScreenDown() {
  notImplemented('clearScreenDown')
}

export function createInterface() { 
  return new Interface()
}

export function cursorTo() {
  notImplemented('cursorTo')
}

export function emitKeypressEvents() {
  notImplemented('emitKeypressEvents')
}

export function moveCursor() {
  notImplemented('moveCursor')
}

export default {
  Interface,
  clearLine,
  clearScreenDown,
  createInterface,
  cursorTo,
  emitKeypressEvents,
  moveCursor,
}
