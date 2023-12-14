import type { Hot } from "../server/embed/types/hot.d.ts";

const html = String.raw;

const template = html`
  <button class="popup" aria-label="Hot Devtools">
    <svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 24 24">
      <path fill="currentColor" d="m8.468 8.395l-.002.001l-.003.002Zm9.954-.187a1.237 1.237 0 0 0-.23-.175a1 1 0 0 0-1.4.411a5.782 5.782 0 0 1-1.398 1.778a8.664 8.664 0 0 0 .134-1.51a8.714 8.714 0 0 0-4.4-7.582a1 1 0 0 0-1.492.806a7.017 7.017 0 0 1-2.471 4.942l-.23.187a8.513 8.513 0 0 0-1.988 1.863a8.983 8.983 0 0 0 3.656 13.908a1 1 0 0 0 1.377-.926a1.05 1.05 0 0 0-.05-.312a6.977 6.977 0 0 1-.19-2.581a9.004 9.004 0 0 0 4.313 4.016a.997.997 0 0 0 .715.038a8.995 8.995 0 0 0 3.654-14.863Zm-3.905 12.831a6.964 6.964 0 0 1-3.577-4.402a8.908 8.908 0 0 1-.18-.964a1 1 0 0 0-.799-.845a.982.982 0 0 0-.191-.018a1 1 0 0 0-.867.5a8.959 8.959 0 0 0-1.205 4.718a6.985 6.985 0 0 1-1.176-9.868a6.555 6.555 0 0 1 1.562-1.458a.745.745 0 0 0 .075-.055s.296-.245.306-.25a8.968 8.968 0 0 0 2.9-4.633a6.736 6.736 0 0 1 1.385 8.088a1 1 0 0 0 1.184 1.418a7.856 7.856 0 0 0 3.862-2.688a7 7 0 0 1-3.279 10.457Z"/>
    </svg>
  </button>
  <a class="site-url" target="_blank" rel="noopener"></a>
  <style>
    button.popup {
      box-sizing: border-box;
      position: fixed;
      bottom: 16px;
      right: 16px;
      display: flex;
      align-items: center;
      justify-content: center;
      width: 36px;
      height: 36px;
      border-radius: 18px;
      border: 1px solid #eee;
      background-color: rgba(255, 255, 255, 0.8);
      backdrop-filter: blur(10px);
      color: rgba(255, 165, 0, 0.9);
      transition: all 0.3s ease;
      cursor: pointer;
    }
    button.popup:focus,
    button.popup:hover,
    button.popup.loading {
      outline: none;
      color: rgba(255, 165, 0, 1);
      border-color: rgba(255, 165, 0, 0.5);
      background-color: rgba(255, 255, 255, 0.9);
      box-shadow: 0 4px 10px 0 rgba(50, 25, 0, 0.1);
    }
    button.popup svg {
      width: 18px;
      height: 18px;
    }
    button.popup.loading {
      pointer-events: none;
      animation: loading 1s infinite;
    }
    @keyframes loading {
      0% {
        transform: rotate(0deg);
      }
      100% {
        transform: rotate(-360deg);
      }
    }
    a.site-url {
      position: fixed;
      bottom: 16px;
      right: 16px;
      display: none;
      align-items: center;
      justify-content: center;
      height: 36px;
      padding: 0 16px;
      border-radius: 18px;
      border: 1px solid #eee;
      background-color: rgba(255, 255, 255, 0.8);
      backdrop-filter: blur(10px);
      color: rgba(0, 200, 55, 0.9);
      transition: all 0.3s ease;
      cursor: pointer;
      text-decoration: none;
    }
    a.site-url:hover {
      color: rgba(0, 200, 55, 1);
      border-color: rgba(0, 200, 55, 0.5);
      background-color: rgba(255, 255, 255, 0.9);
      box-shadow: 0 4px 10px 0 rgba(0, 200, 55, 0.1);
    }
  </style>
`;

export function render(hot: Hot) {
  const d = document;

  class DevTools extends HTMLElement {
    connectedCallback() {
      const root = this.attachShadow({ mode: "open" });
      root.innerHTML = template;
      const button = root.querySelector("button")!;
      const urlBar = root.querySelector("a")!;
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
        urlBar.textContent = `https://${appId}.esm.app`;
        urlBar.href = `https://${appId}.esm.app`;
        urlBar.style.display = "flex";
      };
      button.onclick = () => {
        button.classList.add("loading");
        publish().finally(() => {
          button.classList.remove("loading");
        });
      };
      urlBar.onclick = () => {
        urlBar.style.display = "none";
      };
    }
  }
  customElements.define("hot-devtools", DevTools);
  d.body.appendChild(d.createElement("hot-devtools"));
}
