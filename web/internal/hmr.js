const registry = new Map();
const watchers = new Map();
const messageQueue = [];
const keepAliveTimeout = 30000;
const reload = () => location.reload();

/** @type { WebSocket | null } */
let ws = null;

/** connect to the dev server */
function connect(recoveryMode) {
  const url = "ws" + location.protocol.slice(4) + "//" + location.host + "/@hmr-ws";
  const socket = new WebSocket(url);
  const ping = (callback) => {
    setTimeout(() => {
      const ws = new WebSocket(url);
      ws.addEventListener("open", callback);
      ws.addEventListener("error", () => {
        // retry
        ping(callback);
      });
    }, 500);
  };
  const keepAlive = () => {
    if (ws?.readyState === WebSocket.OPEN) {
      ws.send("ping");
      setTimeout(keepAlive, keepAliveTimeout);
    }
  };
  const colors = {
    create: "#20B44B",
    modify: "#056CF0",
    remove: "#F00C08",
  };

  socket.addEventListener("open", () => {
    ws = socket;
    if (recoveryMode) {
      for (const id of watchers.keys()) {
        socket.send("watch:" + id);
      }
    } else {
      messageQueue.splice(0, messageQueue.length).forEach((msg) => socket.send(msg));
    }
    setTimeout(keepAlive, keepAliveTimeout);
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
          "%c[HMR] %c" + kind,
          "color:#999",
          "color:" + colors[kind],
          JSON.stringify(id),
        );
        watchers.get(id)?.forEach((cb) => cb(kind, id));
        watchers.get("*")?.forEach((cb) => cb(kind, id));
      } else if (command[0] === "error") {
        console.error("[HMR]", command.slice(1).join(":"));
      }
    }
  });
}

/** send message to the dev server */
function sendMessage(msg) {
  if (ws?.readyState === WebSocket.OPEN) {
    ws.send(msg);
  } else {
    messageQueue.push(msg);
  }
}

/** watch for file changes */
function watch(id, callback) {
  if (watchers.has(id)) {
    const callbacks = watchers.get(id);
    const i = callbacks.indexOf(reload);
    if (i >= 0) {
      // remove the default reload callback
      callbacks.splice(i, 1);
    }
    callbacks.push(callback);
  } else {
    watchers.set(id, [callback]);
    if (id !== "*") {
      sendMessage("watch:" + id);
    }
  }
}

class HotContext {
  #url;
  #locked = false;
  constructor(url) {
    this.#url = url;
    watch(url.pathname, reload);
  }
  get locked() {
    return this.#locked;
  }
  lock() {
    this.#locked = true;
    return this;
  }
  accept(callback) {
    if (this.#locked) {
      return;
    }
    watch(this.#url.pathname, (kind) => {
      if (kind === "remove") {
        location.reload();
      } else {
        let url = new URL(this.#url);
        url.searchParams.set("t", Date.now().toString(36));
        import(url.href).then(callback);
      }
    });
  }
  watch(maybeUrl, callback) {
    if (this.#locked) {
      return;
    }
    if (typeof maybeUrl === "function") {
      callback = maybeUrl;
      maybeUrl = undefined;
    }
    if (typeof callback === "function") {
      if (typeof maybeUrl === "string") {
        watch(new URL(maybeUrl, location).pathname, callback);
      } else {
        watch(this.#url.pathname, callback);
      }
    }
  }
}

export default function createHotContext(url) {
  url = new URL(url, location);
  let ctx = registry.get(url.pathname);
  if (ctx) {
    return ctx.lock();
  }
  ctx = new HotContext(url);
  registry.set(url.pathname, ctx);
  return ctx;
}

connect();
