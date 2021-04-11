// https://nodejs.org/api/readline.html

function notImplemented(name) {
  throw new Error(`[esm.sh] readline: '${name}' is not implemented`)
}

export class Interface {
  line
  cursor
  constructor() {
    this.line = ''
    this.cursor = 0
  }
  close() {
    notImplemented('Interface.close')
  }
  pause() {
    notImplemented('Interface.pause')
  }
  prompt() {
    notImplemented('Interface.prompt')
  }
  question() {
    notImplemented('Interface.question')
  }
  resume() {
    notImplemented('Interface.resume')
  }
  setPrompt() {
    notImplemented('Interface.setPrompt')
  }
  getPrompt() {
    notImplemented('Interface.getPrompt')
  }
  write() {
    notImplemented('Interface.write')
  }
  getCursorPos() {
    notImplemented('Interface.getCursorPos')
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
