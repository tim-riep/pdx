// Check for a newer release: at every start and then every six hours while
// the app runs, as long as the check is switched on (UpdateService in Go).
// A new release matters most in its first hours, so the answer is not
// cached across starts. That is far below GitHub's 60 unauthenticated
// requests per hour and address.

import { UpdateAPI, message, type UpdateInfo, type UpdateSettings } from "./api";

const RECHECK_MS = 6 * 60 * 60 * 1000;

export const update = $state({
  info: null as UpdateInfo | null,
  settings: { check: true, skipped: "" } as UpdateSettings,
  // The dialog with the notes of the newer release.
  open: false,
  error: "",
});

let timer: ReturnType<typeof setInterval> | undefined;
// The dialog opens by itself at most once per start.
let shown = false;

async function run() {
  const fresh = await UpdateAPI.Check().catch(() => null);
  // A failed check (offline, rate limit) keeps the last good answer.
  if (!fresh || fresh.error) return;
  update.info = fresh;
  if (fresh.newer && !shown && update.settings.skipped !== fresh.latest) {
    shown = true;
    update.open = true;
  }
}

function schedule() {
  clearInterval(timer);
  timer = undefined;
  if (!update.settings.check) {
    update.info = null;
    return;
  }
  void run();
  timer = setInterval(() => void run(), RECHECK_MS);
}

// startUpdateCheck reads the settings and starts the check.
export async function startUpdateCheck() {
  update.settings = await UpdateAPI.Settings();
  schedule();
}

// setUpdateCheck switches the check on or off.
export async function setUpdateCheck(on: boolean) {
  try {
    update.settings = await UpdateAPI.SetCheck(on);
    update.error = "";
  } catch (e) {
    update.error = message(e);
  }
  schedule();
}

// skipRelease keeps the dialog of this release from opening by itself.
export async function skipRelease() {
  const info = update.info;
  update.open = false;
  if (!info) return;
  try {
    update.settings = await UpdateAPI.Skip(info.latest);
  } catch (e) {
    update.error = message(e);
  }
}

// openRelease shows a release page (default: that of the newer release) in
// the browser of the system, in the server build in a new tab.
export async function openRelease(url = update.info?.url) {
  if (!url) return;
  try {
    await UpdateAPI.Open(url);
    update.error = "";
  } catch (e) {
    const m = message(e);
    if (m.includes("nicht verfügbar")) window.open(url, "_blank", "noopener");
    else update.error = m;
  }
}

// The notes come from GitHub already rendered and sanitized. This is a
// second line of defence before they go into the page: only plain
// formatting elements survive, every attribute except a link's href
// (http and https only) and a few presentational ones is dropped, and
// images and other media are removed, because the app makes no requests
// besides the check itself.
const ALLOWED = new Set([
  "a", "p", "br", "hr", "strong", "b", "em", "i", "code", "tt", "pre", "blockquote",
  "ul", "ol", "li", "h1", "h2", "h3", "h4", "h5", "h6",
  "table", "thead", "tbody", "tr", "th", "td", "del", "sup", "sub", "details", "summary", "span", "div",
]);
const DROP = new Set([
  "script", "style", "img", "picture", "video", "audio", "iframe", "object", "embed",
  "svg", "math", "form", "input", "button", "link", "meta", "template", "noscript",
]);
const KEEP_ATTRS = new Set(["href", "colspan", "rowspan", "align"]);

export function sanitizeNotes(html: string | undefined): string {
  if (!html) return "";
  const doc = new DOMParser().parseFromString(`<div>${html}</div>`, "text/html");
  const root = doc.body.firstElementChild;
  if (!root) return "";
  const walk = (el: Element) => {
    for (const child of Array.from(el.children)) {
      const tag = child.tagName.toLowerCase();
      if (!ALLOWED.has(tag)) {
        // Unknown structure keeps its text, scripts, styles, media and
        // frames go entirely.
        if (DROP.has(tag)) child.remove();
        else child.replaceWith(...Array.from(child.childNodes));
        continue;
      }
      for (const attr of Array.from(child.attributes)) {
        const name = attr.name.toLowerCase();
        if (!KEEP_ATTRS.has(name) || (name === "href" && !/^https?:\/\//i.test(attr.value.trim()))) {
          child.removeAttribute(attr.name);
        }
      }
      walk(child);
    }
  };
  // Repeat until stable: unwrapping an element can expose children the walk
  // has not seen yet.
  let before = "";
  while (before !== root.innerHTML) {
    before = root.innerHTML;
    walk(root);
  }
  return root.innerHTML;
}
