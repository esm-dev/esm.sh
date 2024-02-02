
openMessageChannel(channelName: string): Promise<HotMessageChannel> {
  const url = this.basePath + "@hot-events?channel=" + channelName;
  const conn = new EventSource(url);
  return new Promise((resolve, reject) => {
    const c: HotMessageChannel = {
      onMessage: (handler) => {
        const msgHandler = (evt: MessageEvent) => {
          handler(parse(evt.data));
        };
        conn.addEventListener(kMessage, msgHandler);
        return () => {
          conn.removeEventListener(kMessage, msgHandler);
        };
      },
      postMessage: (data) => {
        return fetch(url, {
          method: "POST",
          body: stringify(data ?? null),
        }).then((res) => res.ok);
      },
      close: () => {
        conn.close();
      },
    };
    conn.onopen = () => resolve(c);
    conn.onerror = () =>
      reject(
        new Error(`Failed to open message channel "${channelName}"`),
      );
  });
}
