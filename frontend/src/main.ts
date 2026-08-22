import { mount } from "svelte";
import "@fontsource-variable/ibm-plex-sans/wght.css";
import "./app.css";
import App from "./App.svelte";
import { initializeTheme } from "./lib/theme";

initializeTheme();

const app = mount(App, {
  target: document.getElementById("app")!,
});

export default app;
