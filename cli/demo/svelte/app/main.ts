import { mount } from "svelte";
import App from "~/App.svelte";

const root = document.getElementById("root")!;

// remove loading placeholder
root.replaceChildren();

// Mount the App component to the root element
mount(App, { target: root });
