// Zugang zu den Go-Services. Die Oberfläche hält kein eigenes Modell, sie
// ruft Methoden auf und hört auf Ereignisse.

import { Events } from "@wailsio/runtime";

export * as AppAPI from "../../bindings/github.com/tim-riep/pdx/designer/internal/app/appservice";
export * as ProjectAPI from "../../bindings/github.com/tim-riep/pdx/designer/internal/app/projectservice";
export * as RenderAPI from "../../bindings/github.com/tim-riep/pdx/designer/internal/app/renderservice";
export * as TemplateAPI from "../../bindings/github.com/tim-riep/pdx/designer/internal/app/templateservice";
export type {
  Snapshot,
  ViewNode,
  ElementType,
  Property,
  Item,
} from "../../bindings/github.com/tim-riep/pdx/designer/internal/model/models";
export type {
  ProjectState,
  RecentFile,
  Recovery,
} from "../../bindings/github.com/tim-riep/pdx/designer/internal/app/models";

// Namen der Ereignisse, wie in internal/app/app.go.
export const EVENT = {
  project: "project:changed",
  render: "render:pages",
  closeRequest: "app:close-request",
} as const;

// on meldet sich für ein Ereignis an und liefert die Abmeldung.
export function on<T>(name: string, cb: (data: T) => void): () => void {
  return Events.On(name as never, (e: { data: unknown }) => cb(e.data as T));
}

// message liefert den Text eines Fehlers aus einem Aufruf.
export function message(err: unknown): string {
  if (err instanceof Error) return err.message;
  if (typeof err === "object" && err && "message" in err) return String((err as { message: unknown }).message);
  return String(err);
}
