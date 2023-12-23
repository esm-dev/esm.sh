import type { Hot } from "../server/embed/types/hot.d.ts";

const html = String.raw;

const template = html`
  <div id="dev-tools">
    <button class="icon dot" tabindex="-1">
      <svg class="dot" width="14" height="14" viewBox="0 0 14 14"  xmlns="http://www.w3.org/2000/svg">
        <circle cx="7" cy="7" r="2" fill="currentColor" />
      </svg>
    </button>
    <ul>
      <li>
        <h1>
        <svg width="12" height="32" viewBox="0 0 12 32" fill="currentColor" xmlns="http://www.w3.org/2000/svg">
          <path d="M6.69387 29.904L11.1879 29.904L11.1879 31.234L0.869865 31.234L0.869865 29.904L5.36387 29.904L5.36387 27.916L0.869865 27.916L0.869865 26.6L11.1879 26.6L11.1879 27.916L6.69387 27.916L6.69387 29.904Z" />
          <path d="M4.46787 21.0355L4.46787 22.3515L3.80987 22.3515C3.20787 22.3515 2.77387 22.4635 2.50787 22.6735C2.43787 22.7155 2.38187 22.7995 2.31187 22.9255C2.24187 23.0515 2.19987 23.2335 2.19987 23.4995C2.19987 24.0035 2.45187 24.2835 2.75987 24.4515C3.06787 24.6055 3.45987 24.6335 3.73987 24.6335C4.22987 24.6335 4.55187 24.4655 4.78987 24.2275C5.01387 23.9895 5.16787 23.6675 5.29387 23.3735L5.40587 23.1215C5.53187 22.8275 5.74187 22.3235 6.17587 21.8755C6.72187 21.3155 7.40787 21.0355 8.24787 21.0355C9.12987 21.0355 9.87187 21.2735 10.3899 21.7215C10.8939 22.1555 11.2019 22.7715 11.2019 23.4995C11.2019 24.5075 10.7679 25.0675 10.3899 25.3615C9.87187 25.7675 9.15787 25.9635 8.24787 25.9635L7.58987 25.9635L7.58987 24.6335L8.24787 24.6335C9.04587 24.6335 9.47987 24.4515 9.67587 24.2135C9.87187 23.9755 9.87187 23.6815 9.87187 23.4995C9.87187 22.9535 9.57787 22.6875 9.21387 22.5335C8.84987 22.3655 8.42987 22.3515 8.24787 22.3515C7.75787 22.3515 7.43587 22.5195 7.18387 22.7575C6.93187 22.9955 6.74987 23.3175 6.62387 23.6535L6.52587 23.8775C6.41387 24.1435 6.23187 24.6475 5.79787 25.0955C5.26587 25.6695 4.57987 25.9635 3.73987 25.9635C2.15787 25.9635 0.883873 25.0815 0.883873 23.4995C0.883873 21.8615 1.96187 21.0355 3.80987 21.0355L4.46787 21.0355Z" />
          <path d="M0.869882 19.7539L0.869882 18.4379L2.35388 18.4379L2.35388 19.7539L0.869882 19.7539Z" />
          <path d="M0.883873 13.6487L5.82587 12.2207L0.883873 12.2207L0.883873 10.9047L11.2019 10.9047L11.2019 12.0667L3.93587 14.1527L11.2019 16.2247L11.2019 17.3867L0.883873 17.3867L0.883873 16.0707L5.82587 16.0707L0.883873 14.6427L0.883873 13.6487Z" />
          <path d="M4.46787 5.34021L4.46787 6.65621C3.53915 6.65621 2.19987 6.55913 2.19987 7.80421C2.19987 9.061 4.04349 9.27859 4.78987 8.53221C5.08278 8.22099 5.23492 7.81085 5.40587 7.42621C5.92618 6.21217 6.85663 5.34021 8.24787 5.34021C9.7147 5.34021 11.2019 6.18311 11.2019 7.80421C11.2019 9.90426 9.31187 10.2682 7.58987 10.2682L7.58987 8.93821C8.51781 8.93821 9.87187 9.05334 9.87187 7.80421C9.87187 6.93161 8.97887 6.65621 8.24787 6.65621C7.42942 6.65621 6.89529 7.23444 6.62387 7.95821C6.39901 8.47219 6.1966 8.98862 5.79787 9.40021C4.27969 11.0383 0.883873 10.2732 0.883873 7.80421C0.883873 5.68557 2.7604 5.34021 4.46787 5.34021Z" />
          <path d="M11.1879 4.40123L9.87187 4.40123L9.87187 1.44723L6.69387 1.44723L6.69387 4.40123L5.37787 4.40123L5.37787 1.44723L2.18587 1.44723L2.18587 4.40123L0.869865 4.40123L0.869865 0.131225L11.1879 0.131226L11.1879 4.40123Z" />
        </svg>
        </h1>
      </li>
      <li>
        <button class="icon">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" xmlns="http://www.w3.org/2000/svg">
            <path d="M1.5 9.25028C8.12486 7.58323 15.8751 7.58325 22.5 9.25028M1.5 14.7497C8.12486 16.4168 15.8751 16.4167 22.5 14.7497" />
            <ellipse cx="12" cy="12" rx="4" ry="11"/>
            <circle cx="12" cy="12" r="11"/>
          </svg>
        </button>
      </li>
      <li>
        <button class="icon">
          <svg class="logo" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" xmlns="http://www.w3.org/2000/svg">
            <path d="M11.4286 2.3158H5.74576C3.41404 2.3158 1.52381 4.0124 1.52381 6.10527C1.52381 8.19814 3.41404 9.89474 5.74576 9.89474H10.6667" />
            <path d="M5.33333 6.10526H10.2542C12.586 6.10526 14.4762 7.80187 14.4762 9.89474C14.4762 11.9876 12.586 13.6842 10.2542 13.6842H4.84033" />
          </svg>
        </button>
      </li>
    </ul>
  </div>
  <style>
    :host {
      --row-height: 27px;
      font-family: ui-sans-serif, system-ui, Inter, sans-serif, "Apple Color Emoji", "Segoe UI Emoji", "Segoe UI Symbol", "Noto Color Emoji";
      font-feature-settings: normal;
      font-variation-settings: normal;
    }
    :host * {
      box-sizing: border-box;
      margin: 0;
      padding: 0;
    }
    :host ul {
      list-style: none;
    }
    button.icon {
      display: flex;
      align-items: center;
      justify-content: center;
      width: var(--row-height);
      height: var(--row-height);
      border-radius: 50%;
      border: none;
      background-color: transparent;
      cursor: pointer;
    }
    button.icon svg {
      width: 13px;
      height: 13px;
    }
    #dev-tools {
      position: fixed;
      bottom: 8px;
      right: 12px;
      z-index: 9999;
    }
    #dev-tools > button.dot {
      position: absolute;
      bottom: 0;
      right: 0;
      opacity: 0.4;
    }
    #dev-tools:has(button:focus) > button.dot,
    #dev-tools:hover > button.dot {
      outline: none;
      opacity: 1;
    }
    #dev-tools > ul {
      position: absolute;
      bottom: calc(var(--row-height) - 8px);
      right: 0;
      display: flex;
      flex-direction: column;
      gap: 4px;
      opacity: 0;
      transition: all 0.24s ease-in-out;
    }
    #dev-tools > ul > li > button {
      background-color: rgba(150, 150, 150, 0.15);
      opacity: 0.8;
    }
    #dev-tools > ul > li > h1 {
      display: flex;
      justify-content: center;
      width: var(--row-height);
      opacity: 0.8;
      padding-bottom: 6px;
    }
    #dev-tools > ul > li:hover > button {
      opacity: 1;
      background-color: rgba(150, 150, 150, 0.25);
    }
    #dev-tools:has(button:focus) > ul,
    #dev-tools:hover > ul {
      opacity: 1;
      bottom: var(--row-height);
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
