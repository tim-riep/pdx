GO      ?= go
OUT     ?= out
PROFILE ?= pdfa-3b
DATE    ?= 2026-01-01T00:00:00Z

CORPUS    := $(wildcard testdata/fo/*.fo)
TEMPLATES := $(wildcard testdata/templates/*/template.xml)
PDFS      := $(patsubst testdata/fo/%.fo,$(OUT)/%.pdf,$(CORPUS)) \
             $(patsubst testdata/templates/%/template.xml,$(OUT)/vorlage-%.pdf,$(TEMPLATES))

.PHONY: all build test vet fuzz golden conformance fop-compare fo-conformance fo-reference dss-image dss image notices notices-check clean

all: vet test

build:
	$(GO) build -o pdx ./cmd/pdx

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

# Rewrite the golden files after an intentional change.
golden:
	UPDATE_GOLDEN=1 $(GO) test ./...

fuzz:
	$(GO) test -run=^$$ -fuzz=FuzzParse -fuzztime=30s ./fo/tree
	$(GO) test -run=^$$ -fuzz=FuzzLoad -fuzztime=30s ./font
	$(GO) test -run=^$$ -fuzz=FuzzTemplateParse -fuzztime=30s ./template
	$(GO) test -run=^$$ -fuzz=FuzzCompile -fuzztime=30s ./compile
	$(GO) test -run=^$$ -fuzz=FuzzExtractData -fuzztime=30s .
	$(GO) test -run=^$$ -fuzz=FuzzSignPDF -fuzztime=30s .
	$(GO) test -run=^$$ -fuzz=FuzzUpload -fuzztime=30s ./server
	$(GO) test -run=^$$ -fuzz=FuzzRender -fuzztime=30s ./server

# Third-party notices with all license texts for pdx and pdx-server. Run
# after every change to the dependencies or embedded files.
notices:
	$(GO) run ./internal/tools/notices -pkgs ./cmd/pdx,./cmd/pdx-server -o THIRD_PARTY_NOTICES.txt

notices-check:
	$(GO) run ./internal/tools/notices -pkgs ./cmd/pdx,./cmd/pdx-server -o THIRD_PARTY_NOTICES.txt -check

# Builds the EU DSS Validator image locally (downloads sources, base images
# and Maven dependencies) and uses it to check all PAdES levels.
dss-image:
	./dssval/build.sh

dss:
	PDX_DSS=1 $(GO) test -run TestSignDSS -v .

# Builds the pdx-server image twice and compares the digests.
image:
	./deploy/build.sh --check

# Renders the corpus and the templates with their first sample data and
# checks every file with veraPDF.
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

# Compares the corpus with Apache FOP (Docker, tests only).
fop-compare:
	PDX_FOP=1 $(GO) test -run 'CorpusAgainstFOP|MusterrechnungAgainstFOP' -v ./internal/fopcompare

# FO test suites (FOP standard-testcases and W3C XSL Test Suite) against the
# expectations from FOP. Downloads the W3C suite on the first run. Report
# in out/fo-conformance.md.
fo-conformance:
	PDX_W3C=1 $(GO) test -run TestImportW3C -count=1 ./internal/fosuite
	PDX_FO_CONFORMANCE=1 $(GO) test -run TestFOConformance -count=1 -v ./internal/fosuite

# Regenerate the test suite expectations with FOP (Docker).
fo-reference:
	PDX_FOP=1 PDX_SUITE_UPDATE=1 $(GO) test -run TestReference -count=1 -timeout 60m -v ./internal/fosuite
