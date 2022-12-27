// https://nodejs.org/api/inspector.html

function notImplemented(name, type = 'function') {
  throw new Error(`[esm.sh] inspector: ${type} '${name}' is not implemented`)
}

export class Session {
  constructor() {
    notImplemented('Session', 'class')
  }
}

export function close() {
  notImplemented('close')
}

export function open() {
  notImplemented('open')
}

export function url() {
  notImplemented('url')
}

export function waitForDebugger() {
  notImplemented('waitForDebugger')
}

export default {
  close,
  console,
  open,
  url,
  waitForDebugger
}
