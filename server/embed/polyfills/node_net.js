// https://nodejs.org/api/net.html

function notImplemented(name) {
  throw new Error(`[esm.sh] net: '${name}' is not implemented`)
}

export class BlockList {
  constructor() {
    notImplemented('BlockList')
  }
}

export class SocketAddress {
  constructor() {
    notImplemented('SocketAddress')
  }
}

export class Server {
  constructor() {
    notImplemented('Server')
  }
}

export class Socket {
  constructor() {
    notImplemented('Socket')
  }
}

export function connect() {
  notImplemented('connect')
}

export function createConnection() {
  notImplemented('createConnection')
}

export function createServer() {
  notImplemented('createServer')
}

export function isIP() {
  notImplemented('isIP')
}

export function isIPv4() {
  notImplemented('isIPv4')
}


export function isIPv6() {
  notImplemented('isIPv6')
}

export default {
  BlockList,
  SocketAddress,
  Server,
  Socket,
  connect,
  createConnection,
  createServer,
  isIP,
  isIPv4,
  isIPv6,
}
