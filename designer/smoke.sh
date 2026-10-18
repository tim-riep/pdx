#!/usr/bin/env bash
# Smoke-Test des nativen Designers in CI: starten, Musterrechnung setzen,
# beenden. Auf dem Windows-Runner fehlt eine interaktive Sitzung, dort
# scheitert WebView2 nach dem Setzen der Seite. Genau dieser Fall gilt als
# Warnung, wenn die Engine die Seite gemeldet hat, jeder andere Fehler zählt.
set -uo pipefail
cd "$(dirname "$0")"
EXT=""; [ "${RUNNER_OS:-}" = "Windows" ] && EXT=".exe"
SMOKE=(../out/pdx-designer$EXT --smoke --open ../testdata/templates/musterrechnung)
log=$(mktemp)
if [ "${RUNNER_OS:-}" = "Linux" ]; then
  # Ohne Benutzernamensräume scheitert die Sandbox von WebKitGTK.
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
