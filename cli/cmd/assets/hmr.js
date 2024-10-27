const registry = new Map();
const watchers = new Map();
const messageQueue = [];

/** @type { WebSocket | null } */
let ws = null;

/** connect to the dev server */
function connect(recoveryMode) {
  const wsUrl = `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/@hmr-ws`;
  const socket = new WebSocket(wsUrl);
  const ping = (callback) => {
    setTimeout(() => {
      const ws = new WebSocket(wsUrl);
      ws.addEventListener("open", callback);
      ws.addEventListener("error", () => {
        // retry
        ping(callback);
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
    if (recoveryMode) {
      for (const ctx of registry.values()) {
        socket.send("watch:" + ctx.id);
      }
    } else {
      messageQueue.splice(0, messageQueue.length).forEach((msg) => socket.send(msg));
    }
    console.log("%c[HMR]", "color:#999", "listening for file changes...");
  });

  socket.addEventListener("close", () => {
    if (ws !== null) {
      ws = null;
      console.log("[HMR] closed.");
      // recovery the connection
      connect(true);
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
        watchers.get(id)?.forEach((cb) => cb(kind, id));
        watchers.get("*")?.forEach((cb) => cb(kind, id));
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

function addWatcher(id, callback) {
  if (typeof id === "string" && typeof callback === "function") {
    if (watchers.has(id)) {
      watchers.get(id).push(callback);
    } else {
      watchers.set(id, [callback]);
    }
  }
}

/**
 * hot context
 */
class HotContext {
  #id;
  #im;
  #locked = false;
  constructor(id, im) {
    this.#id = id;
    this.#im = im;
  }
  get id() {
    return this.#id;
  }
  lock() {
    this.#locked = true;
  }
  accept(callback) {
    if (this.#locked) {
      return;
    }
    addWatcher(this.#id, (kind) => {
      if (kind === "remove") {
        callback();
      } else {
        import(this.#id + "?im=" + this.#im + "&t=" + Date.now().toString(36)).then(callback);
      }
    });
  }
  watch(idOrIdsOrCallback, callback) {
    if (this.#locked) {
      return;
    }
    if (typeof idOrIdsOrCallback === "function") {
      addWatcher(this.#id, idOrIdsOrCallback);
    } else if (Array.isArray(idOrIdsOrCallback)) {
      for (const id of idOrIdsOrCallback) {
        addWatcher(id, callback);
      }
    } else {
      addWatcher(idOrIdsOrCallback, callback);
    }
  }
}

/**
 * create a hot context
 * @param {string} id
 * @returns {HotContext}
 */
export default function createHotContext(id, im) {
  const key = id + "?im=" + im;
  let ctx = registry.get(key);
  if (ctx) {
    ctx.lock();
    return ctx;
  }
  ctx = new HotContext(id, im);
  registry.set(key, ctx);
  sendMessage("watch:" + id);
  return ctx;
}

connect();