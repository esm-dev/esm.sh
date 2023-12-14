import type { Hot } from "../types/hot.d.ts";

export default {
  name: "auto-fire",
  setup(hot: Hot) {
    setTimeout(() => {
      hot.fire();
    }, 0);
  },
};
