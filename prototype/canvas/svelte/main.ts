import { mount } from "svelte";
import "../core/style.css";
import { loadDoc } from "../core/model";
import { installBench } from "../core/bench";
import App from "./App.svelte";

const t0 = performance.now();
const doc = await loadDoc();
mount(App, { target: document.getElementById("app")!, props: { doc } });
requestAnimationFrame(() => installBench("svelte", doc, performance.now() - t0));
