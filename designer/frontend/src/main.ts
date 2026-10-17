import { mount } from "svelte";
import "./theme/tokens.css";
import App from "./App.svelte";

mount(App, { target: document.getElementById("app")! });
