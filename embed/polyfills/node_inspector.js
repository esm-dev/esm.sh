// https://nodejs.org/api/inspector.html

import Events from './node_events_browser.js'

function notImplemented(name) {
  throw new Error(`[esm.sh] inspector: '${name}' is not implemented`)
}

export class Session extends Events {
  connect() {
    notImplemented('Session.connect')
  }
  connectToMainThread() {
    notImplemented('Session.connectToMainThread')
  }
  disconnect() {
    notImplemented('Session.disconnect')
  }
  post() {
    notImplemented('Session.post')
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
