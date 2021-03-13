// https://nodejs.org/api/readline.html

export class Interface {
  line
  cursor
  constructor() {
    this.line = ''
    this.cursor = 0
  }
  close() { }
  pause() { }
  prompt() { }
  question() { }
  resume() { }
  setPrompt() { }
  getPrompt() { }
  write() { }
  getCursorPos() { return { rows: 0, cols: 0 } }
}

export function clearLine() { }
export function clearScreenDown() { }
export function createInterface() { return new Interface() }
export function cursorTo() { }
export function emitKeypressEvents() { }
export function moveCursor() { }

export default {
  Interface,
  clearLine,
  clearScreenDown,
  createInterface,
  cursorTo,
  emitKeypressEvents,
  moveCursor,
}
