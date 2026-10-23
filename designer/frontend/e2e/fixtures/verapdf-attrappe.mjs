// Stand-in for the veraPDF CLI for the end-to-end tests on all systems:
// reports the version and a fixed report with findings at the first
// operators of the first page and one finding without page. It does not
// check the PDF itself. Called via verapdf-attrappe (sh) or
// verapdf-attrappe.cmd (Windows).
if (process.argv.includes("--version")) {
  console.log("veraPDF 1.30.3");
  process.exit(0);
}
const checks = Array.from({ length: 30 }, (_, i) =>
  `<check status="failed"><context>root/document[0]/pages[0](3 0 obj PDPage)/contentStream[0](4 0 obj PDContentStream)/operators[${i}]/fillCS[0]</context><errorMessage>Attrappe am Operator ${i}</errorMessage></check>`,
).join("\n");
process.stdout.write(`<?xml version="1.0" encoding="utf-8"?>
<report><jobs><job><item><name>dokument.pdf</name></item>
<validationReport profileName="PDF/A-3b validation profile" isCompliant="false">
<details passedRules="144" failedRules="2" passedChecks="874" failedChecks="31">
<rule specification="ISO 19005-3:2012" clause="6.6.2.1" testNumber="1" status="failed" failedChecks="1">
<description>Attrappe: Metadaten fehlen</description><object>PDDocument</object><test>containsMetadata == true</test>
<check status="failed"><context>root/document[0]</context><errorMessage>Attrappe ohne Seitenbezug</errorMessage></check>
</rule>
<rule specification="ISO 19005-3:2012" clause="6.2.4.3" testNumber="4" status="failed" failedChecks="30">
<description>Attrappe: Farbraum</description><object>Op_rg_fill</object><test>false</test>
${checks}
</rule></details></validationReport></job></jobs></report>
`);
