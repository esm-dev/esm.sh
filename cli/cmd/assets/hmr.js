const registry = new Map();
const watchers = new Map();
const messageQueue = [];

/** @type {WebSocket|null} */
let ws = null;

/** connect to the dev server */
function connect() {
  const wsUrl = `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/@hmr-ws`;
  const socket = new WebSocket(wsUrl);
  const ping = (callback) => {
    setTimeout(() => {
      const ws = new WebSocket(wsUrl);
      ws.addEventListener("open", callback);
      ws.addEventListener("error", () => {
        ping(callback); // retry
      });
    }, 500);
  };
  const colors = {
    modify: "#056CF0",
    create: "#20B44B",
    remove: "#F00C08",
  };

  socket.addEventListener("open", () => {
    ws = socket;
    messageQueue.splice(0, messageQueue.length).forEach((msg) => socket.send(msg));
    console.log("%c[HMR]", "color:#999", "listening for file changes...");
  });

  socket.addEventListener("close", () => {
    if (ws !== null) {
      ws = null;
      console.log("[HMR] closed.");
      // reconnect
      connect();
    } else {
      // ping to reload the page
      ping(() => location.reload());
    }
  });

  socket.addEventListener("message", ({ data }) => {
    if (typeof data === "string") {
      const command = data.split(":");
      if (command[0] in colors) {
        const [kind, id] = command;
        console.log(
          `%c[HMR] %c${kind}`,
          "color:#999",
          `color:${colors[kind]}`,
          `${JSON.stringify(id)}`,
        );
        watchers.get(id)?.forEach((cb) => cb(kind));
      } else if (command[0] === "error") {
        console.error("[HMR]", command.slice(1).join(":"));
      }
    }
  });
}

/**
 * send message to the dev server
 * @param {string} msg
 */
function sendMessage(msg) {
  if (ws?.readyState === WebSocket.OPEN) {
    ws.send(msg);
  } else {
    messageQueue.push(msg);
  }
}

function addWatcher(id, watchCallback) {
  sendMessage("watch:" + id);
  if (watchers.has(id)) {
    watchers.get(id).push(watchCallback);
  } else {
    watchers.set(id, [watchCallback]);
  }
}

/**
 * hot context
 */
class HotContext {
  #id;
  #locked = false;
  constructor(id) {
    this.#id = id;
  }
  lock() {
    this.#locked = true;
  }
  accept(cb) {
    if (this.#locked) {
      return;
    }
    addWatcher(this.#id, (kind) => kind === "remove" ? cb() : import(this.#id).then(cb));
  }
  watch(filename, cb) {
    if (this.#locked) {
      return;
    }
    addWatcher(filename, cb);
  }
}

/**
 * create a hot context
 * @param {string} id
 * @returns {HotContext}
 */
export default function createHotContext(id) {
  let ctx = registry.get(id);
  if (ctx) {
    ctx.lock();
    return ctx;
  }
  ctx = new HotContext(id);
  registry.set(id, ctx);
  return ctx;
}

connect();
