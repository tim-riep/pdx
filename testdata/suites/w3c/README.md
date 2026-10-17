# W3C XSL Test Suite

Die W3C XSL Test Suite vom 28. August 2001 (XSL 1.0) ist die einzige
Testsuite, die das W3C für XSL veröffentlicht hat. Verwendet werden die
NIST-Tests (XML mit XSLT-Stylesheet, umgewandelt mit go-xslt) und die
FO-Dateien der Beiträge von RenderX, Antenna House und FOP.

Die Tests stammen aus der Candidate Recommendation von XSL 1.0. Der Import
stellt die damals umbenannten Attribute auf die Empfehlung um
(`master-name` wird an `fo:page-sequence` und den Verweisen auf
Seitenvorlagen zu `master-reference`), sonst lehnt FOP 2.11 sie ab.

Die Archive nennen keine Lizenz. pdx verteilt die Dateien deshalb nicht:
`make fo-conformance` lädt sie nach `out/suites/w3c` und prüft die
Prüfsummen (`internal/fosuite/w3c.go`):

| Archiv | SHA-256 |
|---|---|
| https://www.w3.org/Style/XSL/XSL-TS-NIST.zip | `2b34592c37fde21a7b848e549b6b4041e6fb758706e74835be369dd6ab27afb6` |
| https://www.w3.org/Style/XSL/XSL-TS-contrib.zip | `83660625533c299b691e8afaa8c68188b27e0e76e4e116635bbda4547f47c36b` |

Die Erwartungen unter `expect` enthalten nur Seitenzahl, Lage und einen
Hashwert jedes Zeilentextes, keine Inhalte der Testdateien.
