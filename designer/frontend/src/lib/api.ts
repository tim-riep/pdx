// Access to the Go services. The UI keeps no model of its own, it
// calls methods and listens to events.

import { Events } from "@wailsio/runtime";

export * as AppAPI from "../../bindings/github.com/tim-riep/pdx/designer/internal/app/appservice";
export * as ProjectAPI from "../../bindings/github.com/tim-riep/pdx/designer/internal/app/projectservice";
export * as RenderAPI from "../../bindings/github.com/tim-riep/pdx/designer/internal/app/renderservice";
export * as TemplateAPI from "../../bindings/github.com/tim-riep/pdx/designer/internal/app/templateservice";
export * as ConformanceAPI from "../../bindings/github.com/tim-riep/pdx/designer/internal/app/conformanceservice";
export * as SampleAPI from "../../bindings/github.com/tim-riep/pdx/designer/internal/app/sampledataservice";
export * as ValidationAPI from "../../bindings/github.com/tim-riep/pdx/designer/internal/app/validationservice";
export * as SchemaAPI from "../../bindings/github.com/tim-riep/pdx/designer/internal/app/schemaservice";
export * as PublishAPI from "../../bindings/github.com/tim-riep/pdx/designer/internal/app/publishservice";
export type { Schema as DataSchema, Node as DataNode } from "../../bindings/github.com/tim-riep/pdx/designer/internal/schema/models";
export type { Drop, TableOp, Insert, XPathInfo, XPathQuery, Field as DataField, Report, Problem, Conformance, ConformanceFinding, ConformanceItem, TokenOp, ChildInsert, Result, Segment, ContentEdit, Samples, Sample, SchemaDiff, SchemaChange, Rename } from "../../bindings/github.com/tim-riep/pdx/designer/internal/app/models";
export type { Completion, Item as Suggestion, Result as XPathResult } from "../../bindings/github.com/tim-riep/pdx/designer/internal/xpath/models";
export type {
  Snapshot,
  ViewNode,
  ElementType,
  Property,
  Item,
  Command,
  Patch,
  History,
  Child,
} from "../../bindings/github.com/tim-riep/pdx/designer/internal/model/models";
export type { Access } from "../../bindings/github.com/tim-riep/pdx/designer/internal/pipeline/models";
export type { StructNode } from "../../bindings/github.com/tim-riep/pdx/pdf/render/models";
export type {
  ProjectState,
  RecentFile,
  Recovery,
  VeraPDFSetup,
  PublishState,
  PublishDiff,
  PublishResult,
  FileChange,
  DiffLine,
} from "../../bindings/github.com/tim-riep/pdx/designer/internal/app/models";

// Names of the events, as in internal/app/app.go.
export const EVENT = {
  project: "project:changed",
  render: "render:pages",
  template: "template:changed",
  problems: "problems:changed",
  conformance: "conformance:changed",
  closeRequest: "app:close-request",
} as const;

// on subscribes to an event and returns the unsubscribe function.
export function on<T>(name: string, cb: (data: T) => void): () => void {
  return Events.On(name as never, (e: { data: unknown }) => cb(e.data as T));
}

// message returns the text of an error from a call.
export function message(err: unknown): string {
  if (err instanceof Error) return err.message;
  if (typeof err === "object" && err && "message" in err) return String((err as { message: unknown }).message);
  return String(err);
}
