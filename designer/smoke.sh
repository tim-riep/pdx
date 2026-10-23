#!/usr/bin/env bash
# Smoke test of the native designer in CI: start, typeset the sample invoice,
# quit. The Windows runner has no interactive session, so WebView2 fails
# there after the page has been typeset. Exactly this case counts as a
# warning if the engine has reported the page, every other error counts.
set -uo pipefail
cd "$(dirname "$0")"
EXT=""; [ "${RUNNER_OS:-}" = "Windows" ] && EXT=".exe"
SMOKE=(../out/pdx-designer$EXT --smoke --open ../testdata/templates/musterrechnung)
log=$(mktemp)
if [ "${RUNNER_OS:-}" = "Linux" ]; then
  # Without user namespaces the sandbox of WebKitGTK fails.
  WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1 xvfb-run -a "${SMOKE[@]}" 2>&1 | tee "$log"
else
  "${SMOKE[@]}" 2>&1 | tee "$log"
fi
code=${PIPESTATUS[0]}
if [ "$code" -ne 0 ] && [ "${RUNNER_OS:-}" = "Windows" ] \
  && grep -q "smoke: [1-9][0-9]* Seiten" "$log" && grep -q "WebView2 controller did not complete" "$log"; then
  echo "::warning::WebView2 startet auf dem Windows-Runner nicht (keine interaktive Sitzung), die Engine hat die Seite gesetzt."
  code=0
fi
rm -f "$log"
exit "$code"
