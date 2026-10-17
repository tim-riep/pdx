// Zustand des geöffneten Projekts als Lesecache. Änderungen laufen immer über
// den ProjectService, der neue Zustand kommt als Ereignis zurück.

import { EVENT, ProjectAPI, message, on, type ProjectState } from "./api";
import { t } from "./i18n";

export const project: ProjectState = $state({ open: false, path: "", name: "", format: "", dirty: false, rev: 0 });

export const ui = $state({
  error: "",
  notice: "",
  // pathPrompt fragt im Browser nach einem Pfad, wenn es keine Dateidialoge gibt.
  pathPrompt: null as null | { title: string; resolve: (path: string | null) => void },
  closeRequest: false,
});

function apply(s: ProjectState) {
  Object.assign(project, s);
}

on<ProjectState>(EVENT.project, apply);
on<ProjectState>(EVENT.closeRequest, () => (ui.closeRequest = true));
ProjectAPI.State().then(apply);

async function run(f: () => Promise<ProjectState>): Promise<boolean> {
  try {
    apply(await f());
    ui.error = "";
    return true;
  } catch (e) {
    ui.error = message(e);
    return false;
  }
}

function askPath(title: string): Promise<string | null> {
  return new Promise((resolve) => (ui.pathPrompt = { title, resolve }));
}

// Ohne native Dialoge (Server-Build im Browser) wird nach dem Pfad gefragt.
function noDialogs(e: unknown): boolean {
  return message(e).includes("nicht verfügbar");
}

export const newProject = () => run(() => ProjectAPI.New());

export async function openProject(dir = false): Promise<boolean> {
  try {
    apply(await ProjectAPI.OpenDialog(dir));
    ui.error = "";
    return true;
  } catch (e) {
    if (!noDialogs(e)) {
      ui.error = message(e);
      return false;
    }
    const path = await askPath(t("path.open"));
    return path ? run(() => ProjectAPI.Open(path)) : false;
  }
}

export const openPath = (path: string) => run(() => ProjectAPI.Open(path));

export async function saveAs(): Promise<boolean> {
  try {
    apply(await ProjectAPI.SaveAsDialog());
    ui.error = "";
    return !project.dirty;
  } catch (e) {
    if (!noDialogs(e)) {
      ui.error = message(e);
      return false;
    }
    const path = await askPath(t("path.save"));
    return path ? run(() => ProjectAPI.SaveAs(path)) : false;
  }
}

export async function save(): Promise<boolean> {
  if (!project.path) return saveAs();
  const ok = await run(() => ProjectAPI.Save());
  if (ok) ui.notice = t("project.saved");
  return ok;
}

export const recover = (id: string) => run(() => ProjectAPI.Recover(id));
