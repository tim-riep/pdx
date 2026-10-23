# W3C XSL Test Suite

The W3C XSL Test Suite of 28 August 2001 (XSL 1.0) is the only test suite
the W3C has published for XSL. pdx uses the NIST tests (XML with an XSLT
stylesheet, transformed with go-xslt) and the FO files of the contributions
from RenderX, Antenna House and FOP.

The tests come from the Candidate Recommendation of XSL 1.0. The import
switches the attributes that were renamed at the time to the Recommendation
(`master-name` on `fo:page-sequence` and on references to page masters
becomes `master-reference`), otherwise FOP 2.11 rejects them.

The archives state no license. For this reason pdx does not distribute the
files: `make fo-conformance` downloads them to `out/suites/w3c` and verifies
the checksums (`internal/fosuite/w3c.go`):

| Archive | SHA-256 |
|---|---|
| https://www.w3.org/Style/XSL/XSL-TS-NIST.zip | `2b34592c37fde21a7b848e549b6b4041e6fb758706e74835be369dd6ab27afb6` |
| https://www.w3.org/Style/XSL/XSL-TS-contrib.zip | `83660625533c299b691e8afaa8c68188b27e0e76e4e116635bbda4547f47c36b` |

The expectations under `expect` contain only the page number, the position
and a hash of each line text, no content of the test files.
