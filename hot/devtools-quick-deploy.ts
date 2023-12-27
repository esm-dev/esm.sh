import type { DevtoolsWidget, Hot } from "../server/embed/types/hot.d.ts";

const html = String.raw;
const component = "hot-quick-deploy";

const icon = html`
  <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" xmlns="http://www.w3.org/2000/svg">
    <path d="M1.5 9.25028C8.12486 7.58323 15.8751 7.58325 22.5 9.25028M1.5 14.7497C8.12486 16.4168 15.8751 16.4167 22.5 14.7497" />
    <ellipse cx="12" cy="12" rx="4" ry="11"/>
    <circle cx="12" cy="12" r="11"/>
  </svg>
`;

const template = html`
  <div>
    quick deploy
  <div>
  <style>
    :host * {
      box-sizing: border-box;
      margin: 0;
      padding: 0;
      line-height: 1;
    }
  </style>
`;

export default (hot: Hot): DevtoolsWidget => {
  class QuickDeploy extends HTMLElement {
    connectedCallback() {
      const root = this.attachShadow({ mode: "open" });
      root.innerHTML = template;

      const publish = async () => {
        const res = fetch(new URL(hot.basePath + "@hot-index", location.href));
        if (!res) {
          return;
        }
        const index = await res.then((r) => r.json());
        if (!Array.isArray(index) || index.length === 0) {
          return;
        }
        index.push(
          ...index.filter((name: string) => name.endsWith(".css"))
            .map((name: string) => name + "?module"),
        );
        const loader: Record<string, string> = {};
        const fd = new FormData();
        await Promise.all(index.map(async (name: string) => {
          const res = await fetch(
            new URL(hot.basePath + name, location.href),
            { headers: { "hot-loader-env": "production" } },
          );
          if (!res) {
            return;
          }
          if (res.headers.get("x-content-source") === "hot-loader") {
            loader[name] = res.headers.get("content-type")!;
          }
          fd.append(name, await res.blob());
        }));
        fd.append("index", JSON.stringify(index));
        fd.append("loader", JSON.stringify(loader));
        const res2 = await fetch("https://esm.sh/create/x-site", {
          method: "POST",
          body: fd,
        });
        if (!res) {
          return;
        }
        const { appId } = await res2.json();
        alert(`https://${appId}.esm.app`);
      };
    }
  }

  customElements.define(component, QuickDeploy);
  return { icon, component };
};
