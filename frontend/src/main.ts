import { mount } from "svelte";
import "@fontsource-variable/ibm-plex-sans/wght.css";
import "./app.css";
import Root from "./Root.svelte";
import { initializeTheme } from "./lib/theme";

initializeTheme();

mount(Root, {
  target: document.getElementById("app")!,
});
