import type { Hot } from "../server/embed/types/hot.d.ts";

export default {
  name: "auto-fire",
  setup(hot: Hot) {
    setTimeout(() => {
      hot.fire();
    }, 0);
  },
};
