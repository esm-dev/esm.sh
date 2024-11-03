import { render } from "preact";
import { App } from "~/App.tsx";

const root = document.getElementById("root")!;
root.replaceChildren(); // remove loading placeholder

render(<App />, root);
