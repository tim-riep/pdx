# Designer bauen und testen. Das Frontend wird vor jedem Go-Build gebaut,
# weil main.go frontend/dist einbettet.
GO ?= go
WAILS := $(GO) run github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.30
OUT := ../out
EXE := $(if $(filter Windows_NT,$(OS)),.exe,)
# macOS: gleiche Mindestversion für Go und die Objective-C-Teile von Wails.
# Nur dort, gcc unter Linux und Windows kennt die Option nicht.
ifeq ($(shell uname -s 2>/dev/null),Darwin)
export MACOSX_DEPLOYMENT_TARGET ?= 11.0
export CGO_CFLAGS ?= -mmacosx-version-min=$(MACOSX_DEPLOYMENT_TARGET)
export CGO_LDFLAGS ?= -mmacosx-version-min=$(MACOSX_DEPLOYMENT_TARGET)
endif

.PHONY: all deps bindings frontend build bundle package server test check e2e smoke clean notices notices-check

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
	$(GO) run ../internal/tools/notices -cgo -npm frontend/bundled-packages.json -node frontend/node_modules -title "pdx Designer" -o notices/THIRD_PARTY_NOTICES.txt

# notices-check scheitert, wenn die Hinweise nicht zum Stand passen (CI).
notices-check: notices
	git diff --exit-code -- notices/THIRD_PARTY_NOTICES.txt frontend/bundled-packages.json

build: frontend
	$(GO) build -o $(OUT)/pdx-designer$(EXE) .

VERSION := $(shell sed -n 's/^const Version = "\(.*\)"/\1/p' internal/app/app.go)
APP := $(OUT)/pdx Designer.app
PKG := $(OUT)/package/pdx-designer

# macOS: App-Bündel aus dem nativen Binary.
bundle: build
	rm -rf "$(APP)"
	mkdir -p "$(APP)/Contents/MacOS" "$(APP)/Contents/Resources"
	cp $(OUT)/pdx-designer "$(APP)/Contents/MacOS/pdx-designer"
	sed 's/@VERSION@/$(VERSION)/g' packaging/Info.plist.in > "$(APP)/Contents/Info.plist"
	printf 'APPL????' > "$(APP)/Contents/PkgInfo"
	cp ../LICENSE ../NOTICE notices/THIRD_PARTY_NOTICES.txt "$(APP)/Contents/Resources/"

# Inhalt des Release-Archivs für dieses System mit Installationsskript nach
# der Konvention pro Benutzer (siehe README).
ifeq ($(OS),Windows_NT)
package: build
	rm -rf "$(PKG)" && mkdir -p "$(PKG)"
	cp $(OUT)/pdx-designer.exe packaging/install.ps1 "$(PKG)/"
else ifeq ($(shell uname -s),Darwin)
package: bundle
	rm -rf "$(PKG)" && mkdir -p "$(PKG)"
	cp -R "$(APP)" packaging/install.sh "$(PKG)/"
else
package: build
	rm -rf "$(PKG)" && mkdir -p "$(PKG)"
	cp $(OUT)/pdx-designer packaging/install.sh "$(PKG)/"
endif
	cp ../LICENSE ../NOTICE notices/THIRD_PARTY_NOTICES.txt README.md "$(PKG)/"

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
	rm -rf frontend/dist $(OUT)/pdx-designer$(EXE) $(OUT)/designer-server$(EXE) "$(APP)" $(OUT)/package
