# Designer bauen und testen. Das Frontend wird vor jedem Go-Build gebaut,
# weil main.go frontend/dist einbettet.
GO ?= go
WAILS := $(GO) run github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.30
OUT := ../out
EXE := $(if $(filter Windows_NT,$(OS)),.exe,)
# macOS: gleiche Mindestversion für Go und die Objective-C-Teile von Wails.
export MACOSX_DEPLOYMENT_TARGET ?= 11.0
export CGO_CFLAGS ?= -mmacosx-version-min=$(MACOSX_DEPLOYMENT_TARGET)
export CGO_LDFLAGS ?= -mmacosx-version-min=$(MACOSX_DEPLOYMENT_TARGET)

.PHONY: all deps bindings frontend build server test check e2e smoke clean notices notices-check

all: build

deps:
	cd frontend && npm ci --ignore-scripts

bindings:
	$(WAILS) generate bindings -ts -d frontend/bindings

frontend:
	cd frontend && npx vite build

# Drittanbieterhinweise aus den gelinkten Go-Modulen und den gebündelten
# npm-Paketen (frontend/bundled-packages.json schreibt der Vite-Build).
notices: frontend
	$(GO) run ./tools/notices

# notices-check scheitert, wenn die Hinweise nicht zum Stand passen (CI).
notices-check: notices
	git diff --exit-code -- notices/THIRD_PARTY_NOTICES.txt frontend/bundled-packages.json

build: frontend
	$(GO) build -o $(OUT)/pdx-designer$(EXE) .

server: frontend
	CGO_ENABLED=0 $(GO) build -tags server -o $(OUT)/designer-server$(EXE) .

test:
	CGO_ENABLED=0 $(GO) test ./internal/...
	cd frontend && npx vitest run --passWithNoTests

check: frontend
	$(GO) vet ./...
	cd frontend && npx svelte-check --tsconfig ./tsconfig.json --fail-on-warnings

e2e: frontend
	cd frontend && npx playwright test

smoke: build
	$(OUT)/pdx-designer$(EXE) --smoke --open ../testdata/templates/musterrechnung

clean:
	rm -rf frontend/dist $(OUT)/pdx-designer$(EXE) $(OUT)/designer-server$(EXE)
