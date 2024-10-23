import { render } from "preact";
import { App } from "./App.tsx";

const root = document.getElementById("root")!;
root.replaceChildren();

render(<App />, root);
