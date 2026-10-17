import { createRoot } from "react-dom/client";
import "../core/style.css";
import { loadDoc } from "../core/model";
import { installBench } from "../core/bench";
import { App } from "./App";

const t0 = performance.now();
const doc = await loadDoc();
createRoot(document.getElementById("app")!).render(<App doc={doc} />);
requestAnimationFrame(() => requestAnimationFrame(() => installBench("react", doc, performance.now() - t0)));
