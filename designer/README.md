# pdx Designer

Desktop application for laying out documents against a data XSD without writing XML. It is built on the pdx engine (parent directory). It saves only in template format 1, lossless and with stable Git diffs. The result is a PDF in the profile of the template, by default PDF/A-3b with embedded data. The user interface is in German.

The sample invoice can be rebuilt from scratch in the Designer and passes veraPDF 1.30.3 against PDF/A-3b (end-to-end test `e2e/rebuild.spec.ts`).

## Installation

Releases contain setup programs (macOS `.pkg`, Windows setup `.exe`, Linux `.deb`) and archives with an install script. pdx Designer is installed per user, without administrator rights. Only the Debian package installs system-wide (`/usr/bin/pdx-designer`) and pulls in GTK 4 and WebKitGTK 6.

| System | Program | Menu entry | Settings and autosaves |
|---|---|---|---|
| macOS | `~/Applications/pdx Designer.app` | Launchpad, Spotlight | `~/Library/Application Support/pdx-designer` |
| Linux | `~/.local/bin/pdx-designer` | `~/.local/share/applications/pdx-designer.desktop` | `~/.config/pdx-designer` |
| Windows | `%LOCALAPPDATA%\Programs\pdx\pdx-designer.exe` | Start menu, pdx Designer | `%APPDATA%\pdx-designer` |

```bash
./install.sh
```

```powershell
powershell -ExecutionPolicy Bypass -File install.ps1
```

Running the script again updates the installation, `--uninstall` (`-Uninstall` on Windows) removes program and menu entry and keeps settings and autosaves. The Windows setup uninstalls through Settings, Apps, on macOS move the app to the trash.

Everything lives in `packaging/`:

| File | Purpose |
| --- | --- |
| `Info.plist.in` | metadata of the macOS app bundle |
| `install.sh`, `install.ps1` | per-user install scripts of the archives |
| `build-pkg.sh`, `macos/` | macOS installer package (per user, domain currentUserHome, no administrator rights) |
| `windows/pdx-designer.iss` | Windows setup with Inno Setup 6 (per user, Start menu, WebView2 check) |
| `build-deb.sh` | Debian package |
| `test-install.sh`, `test-installer.sh` | CI: install, check and uninstall on all three systems |

`make package` assembles the archive content, `make installer` builds the setup program for the current system into `../out/installer` (on Windows with `ISCC=<path to ISCC.exe>`).

veraPDF: the conformance panel shows under veraPDF which program the Designer uses. Without a path it searches the `PATH`, `~/verapdf`, `/usr/local/bin`, Homebrew and on Windows the user folder and Program Files, then Docker. A path set there is saved in `verapdf.json` in the settings directory and takes precedence over `PDX_VERAPDF`.

## Concept

The window has three columns with tabs and a panel at the bottom.

| Area | Tabs | Purpose |
| --- | --- | --- |
| left | Daten, Hierarchie, Beispiele | data tree of the XSD for dragging, tree of the template, sample data and edge cases |
| center | Entwurf, Vorschau, PDF | canvas with placeholders or sample data, the finished PDF with pdf.js |
| right | Inspektor, Stile | properties of the selected element, design tokens and styles |
| bottom | Probleme, Konformität, Barrierefreiheit, XPath | live checks, veraPDF, structure tree and reading order, large XPath editor |

Principles:

* **A click selects the template element.** All occurrences in the typeset document (for example a table row on every page) are highlighted. A click in the hierarchy, the problem list or the conformance panel leads to the element.
* **Drag data instead of typing paths.** A data node from the Daten tab becomes the matching element when dropped: dates and decimals become fields with formats, repeated nodes become tables, groups become bound subforms, optional nodes get a presence condition.
* **Building blocks** (text, positioned or flowing subform, line, rectangle, image, barcode, page number, table, form fields) come from the palette above the canvas, by click into the selection or by dragging to a position.
* **Text with fields** is edited in place: double click in the canvas or the content editor in the inspector. Fields are chips in the text, `@` suggests data nodes.
* **Every change is a command** with undo and redo. The model lives only in the backend, the user interface shows a projection.
* **Checks run continuously.** After every change the problem list reports errors from template, translations, bindings and sample data. The conformance panel validates with veraPDF against every standard of the output profile on request and maps findings to elements through page and operator.

More tools:

* **Structure:** the context menu in hierarchy and canvas inserts every allowed child (for example formats, text resources, page masters with header and footer, attachments), duplicates, moves and deletes.
* **Tables:** insert columns and rows, header and footer with repetition, cells spanning columns, subtotal, carry-over and total over numeric columns.
* **XPath:** fields in the inspector with suggestions and a check line (value with the sample data), the XPath tab as a large editor, presence through a choice (always, if present, if not empty, custom expression).
* **Tokens and styles:** name colors, lengths and fonts and use them with `$name`, renaming updates all references.
* **Schema change:** a new XSD first shows the differences, moves bindings to renamed nodes and leaves a task list for the rest.
* **Sample data:** import, select, generate edge cases (minimal, maximal, long texts).
* **Output profile:** the template chooses its profile from the valid combinations, for example PDF/A-3a with PDF/UA-1 or PDF/A-4f with PDF/UA-2, each with a short explanation.
* **Accessibility:** the inspector has its own group for role, alternate text (fixed or from the data), language, reading order, decoration and header references. The Barrierefreiheit panel shows the findings of the pre-check and the structure tree in reading order, overlays the order as numbers on the page, moves freely placed elements with arrows and lists the manual checkpoints of Matterhorn 1.1 as a checklist. If the profile does not require accessibility, it checks against PDF/A-3a with PDF/UA-1 for preview.
* **E-invoice:** on the e-invoice element the inspector shows the level detected from the sample data, the embedded file and the relationship, and warns if the profile is not PDF/A-3.
* **Forms:** text fields, check boxes, radio groups, lists and signature fields from the palette, options from the XSD enumeration, fillable in the preview, tab order as numbers on the page.
* **Publish:** the Veröffentlichen dialog connects to a pdx-server (address and API key with the templates right), compares the template with the active version on the server file by file and `template.xml` line by line, uploads it as a new version and activates it. The key stays in memory for the session only, only the address is saved.
* **Licenses:** the Lizenzen button in the toolbar (also in the About dialog and the command palette) shows the Designer's own license and every included component with its license and full text, with search and copy.
* **View:** rulers in mm or pt, grid, snapping to edges and centers, page overview, align and distribute with multiple selection (Shift+click).

## Keyboard shortcuts

On macOS, ⌘ stands for Ctrl and ⌥ for Alt.

| Shortcut | Action |
| --- | --- |
| Ctrl+K or Ctrl+Shift+P | command palette with all actions and search for elements |
| Ctrl+N, Ctrl+O, Ctrl+S, Ctrl+Shift+S | new, open, save, save as |
| Ctrl+Z, Ctrl+Shift+Z or Ctrl+Y | undo, redo |
| Ctrl+D | duplicate |
| Delete or Backspace | delete |
| Alt+Up or Down | move among siblings |
| arrow keys | move a positioned element by 1 mm or 1 pt, with Shift by 10 |
| arrow keys in the hierarchy | select a row, Right expands, Left collapses or goes to the parent |
| Shift+F10 or context menu key | context menu of the element |
| double click on text | edit text in place, Enter applies, Escape discards |
| @ in text | insert a field, Shift+Enter inserts a line break |
| Ctrl+Space in an XPath field | suggestions, Ctrl+Enter applies in the XPath tab |
| Escape | close a menu, otherwise clear the selection |

## Architecture

```
designer/
  main.go               the only place with Wails (and cgo in the app binary)
  internal/app/         services as plain Go types, without Wails, cgo free
  internal/model/       document, commands with inverses, catalog from the XSD, projection and patches
  internal/pipeline/    typesetting with caches, pages as compact JSON, PDF with render trace
  internal/schema/      data tree of the data XSD, binding context, placeholders and edge cases
  internal/xpath/       check, evaluate (go-xslt) and complete paths
  notices/              third-party notices (make notices)
  packaging/            macOS bundle metadata, install scripts and install test
  frontend/             Svelte 5, TypeScript, Vite, everything bundled locally
```

* **One source of truth:** the template (`template.Template`) lives only in the backend. The user interface sends commands (`setAttr`, `insert`, `remove`, `move`, `content`, `batch`, `setFile`) and receives patches of a projection through the event `template:changed`. After every command the backend validates against the format XSD and the references in the package and reverts the command otherwise.
* **No layout in the frontend:** everything visible comes from the area tree of the engine. Every area carries the template ID, selection, hit testing, guides and the mapping of findings rely on it.
* **Services:** AppService (info, licenses), ProjectService (open, save, autosave every 30 s, recovery), TemplateService (commands, tables, structure, content, tokens, XPath), RenderService (view, pages, fonts, images and PDF through `/pdx/`), SchemaService (data tree, import, compare and replace), SampleDataService, ValidationService, ConformanceService and PublishService.
* **Pipeline:** 80 ms after the last change the backend typesets again. The compiler only runs when the content changed, a new run cancels the previous one. Only pages with a new hash are transferred.
* **Checks:** the same checks as `pdx check` (engine package `check`), plus paths in presence conditions. veraPDF runs locally or in Docker in the background.

Licenses: the Designer is licensed under the Apache License 2.0. Everything included is under MIT, BSD, Apache 2.0 or, for the embedded Liberation fonts, the SIL Open Font License 1.1, the hyphenation patterns under MIT and free redistribution with notice. `make notices` generates the full texts with the shared tool `../internal/tools/notices` into `notices/THIRD_PARTY_NOTICES.txt`.

## Performance

Measured on an Apple M4 with the sample invoice (60 line items, three pages).

| Measurement | Result | Target | Where |
| --- | --- | --- | --- |
| command to patch, p95 | 4.8 ms | 20 ms | `BenchmarkCommandPatch` |
| typeset again after a change, mean | 58 ms | 300 ms | `BenchmarkDesignerEdit` |
| edit to new page in the canvas, p95 | 200 ms | 300 ms | `e2e/perf.spec.ts` (with 80 ms debounce) |
| dragging, frame interval p95 | 16.8 ms (60 fps) | 33 ms | `e2e/perf.spec.ts` |
| open to first page | 160 ms | 1.5 s | `e2e/perf.spec.ts` |
| app start to first page | 280 ms | 1.5 s | `--smoke` |

```bash
go test -run '^$' -bench . ./internal/...
```

```bash
PDX_PERF=1 npx playwright test e2e/perf.spec.ts --project chromium
```

## Build and test

Requirements: Go 1.26, Node 22 (22.22.2 or later) or Node 24 (24.15.0 or later). On Linux GTK 4.10 or later and WebKitGTK 6.0 (`libgtk-4-dev libwebkitgtk-6.0-dev`, for example Ubuntu 24.04 or Debian 13). In containers WebKitGTK needs `WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1`.

```bash
make deps
```

```bash
make build
```

| Target | Effect |
| --- | --- |
| `make build` | build the frontend, native binary to `../out/pdx-designer` |
| `make bundle` | macOS: app bundle `../out/pdx Designer.app` |
| `make package` | archive content for this system with install script in `../out/package/pdx-designer` |
| `make installer` | setup program for this system in `../out/installer` (`.pkg`, `.deb`, Inno Setup `.exe`) |
| `make server` | server build without window and without cgo (for Playwright) |
| `make test` | Go tests of the services (`CGO_ENABLED=0`) and Vitest |
| `make check` | `go vet` and svelte-check |
| `make e2e` | Playwright in Chromium and WebKit against the server build |
| `make smoke` | start the native binary, typeset the sample invoice, quit |
| `make bindings` | regenerate the TypeScript bindings of the services |
| `make notices` | rewrite the third-party notices, `notices-check` checks them |

The end-to-end tests use a veraPDF stand-in. With real veraPDF (local 1.30.3 or Docker):

```bash
PDX_E2E_REAL_VERAPDF=1 npx playwright test e2e/rebuild.spec.ts --project chromium
```

Command line options of the binary: `--open <package>` opens a package, `--smoke` typesets it and quits (exit code 1 on errors). `PDX_DESIGNER_DATA` replaces the data directory for settings and autosaves, `PDX_VERAPDF` selects veraPDF (`docker`, `local` or a path).

CI (`.github/workflows/ci.yml`) builds on macOS, Windows and Linux, runs the smoke test, checks the installation and runs all tests.
