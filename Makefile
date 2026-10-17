GO      ?= go
OUT     ?= out
PROFILE ?= pdfa-3b
DATE    ?= 2026-01-01T00:00:00Z

CORPUS    := $(wildcard testdata/fo/*.fo)
TEMPLATES := $(wildcard testdata/templates/*/template.xml)
PDFS      := $(patsubst testdata/fo/%.fo,$(OUT)/%.pdf,$(CORPUS)) \
             $(patsubst testdata/templates/%/template.xml,$(OUT)/vorlage-%.pdf,$(TEMPLATES))

.PHONY: all build test vet fuzz golden conformance fop-compare clean

all: vet test

build:
	$(GO) build -o pdx ./cmd/pdx

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

# Golden Files nach bewusster Änderung neu schreiben.
golden:
	UPDATE_GOLDEN=1 $(GO) test ./...

fuzz:
	$(GO) test -run=^$$ -fuzz=FuzzParse -fuzztime=30s ./fo/tree
	$(GO) test -run=^$$ -fuzz=FuzzLoad -fuzztime=30s ./font
	$(GO) test -run=^$$ -fuzz=FuzzTemplateParse -fuzztime=30s ./template
	$(GO) test -run=^$$ -fuzz=FuzzCompile -fuzztime=30s ./compile

# Rendert den Korpus und die Vorlagen mit ihren ersten Beispieldaten und prüft
# jede Datei mit veraPDF.
conformance: build $(PDFS)
	@fail=0; for f in $(PDFS); do \
		echo "== $$f"; ./pdx verify $$f --profile $(PROFILE) || fail=1; \
	done; exit $$fail

$(OUT)/%.pdf: testdata/fo/%.fo build
	@mkdir -p $(OUT)
	./pdx render $< -o $@ --profile $(PROFILE) --creation-date $(DATE)

$(OUT)/vorlage-%.pdf: build
	@mkdir -p $(OUT)
	./pdx check testdata/templates/$*
	./pdx render --template testdata/templates/$* $$(ls testdata/templates/$*/samples/*.xml | head -1) \
		-o $@ --strict --profile $(PROFILE) --creation-date $(DATE)

clean:
	rm -rf $(OUT) pdx

# Vergleicht den Korpus mit Apache FOP (Docker, nur für Tests).
fop-compare:
	PDX_FOP=1 $(GO) test -run 'CorpusAgainstFOP|MusterrechnungAgainstFOP' -v ./internal/fopcompare
