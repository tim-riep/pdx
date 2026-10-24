#!/usr/bin/env bash
# Signs Windows executables with the Certum Open Source Code Signing
# certificate (SimplySign cloud) through ssign and verifies the signatures.
#
#   packaging/windows/sign.sh <description> <file.exe>…
#
# Environment: CERTUM_EMAIL, CERTUM_OTP (TOTP seed, base32, ssign derives
# the code), optional SSIGN (path to ssign). The output of ssign and of the
# verification names the certificate holder and stays out of the log.
set -euo pipefail
[ $# -ge 2 ] || { echo "Aufruf: $0 <Beschreibung> <Datei.exe>…" >&2; exit 2; }
: "${CERTUM_EMAIL:?CERTUM_EMAIL fehlt}"
: "${CERTUM_OTP:?CERTUM_OTP fehlt}"
desc=$1
shift
ssign=${SSIGN:-ssign}
log=$(mktemp)
trap 'rm -f "$log"' EXIT

# A one-time code is accepted once per 30 second window. Jobs signing at the
# same moment can collide, so a failed attempt waits for the next window.
ok=0
for try in 1 2 3; do
  if "$ssign" -n "$desc" -u https://github.com/tim-riep/pdx "$@" >"$log" 2>&1; then
    ok=1
    break
  fi
  echo "sign: Versuch $try fehlgeschlagen" >&2
  [ "$try" = 3 ] || sleep 35
done
if [ "$ok" = 0 ]; then
  grep -viE 'subject|issuer|cn=' "$log" >&2 || true
  exit 1
fi

for f in "$@"; do
  case "$(uname -s)" in
  MINGW* | MSYS* | CYGWIN*)
    status=$(powershell -NoProfile -Command "\$s = Get-AuthenticodeSignature -LiteralPath '$(cygpath -w "$f")'; if (\$s.Status -eq 'Valid' -and \$s.TimeStamperCertificate) { 'ok' } else { \$s.Status }")
    ;;
  *)
    ca=/etc/ssl/certs/ca-certificates.crt
    [ -f "$ca" ] || ca="$(brew --prefix 2>/dev/null)/etc/ca-certificates/cert.pem"
    if osslsigncode verify -CAfile "$ca" -TSA-CAfile "$ca" -in "$f" >/dev/null 2>&1; then status=ok; else status=ungueltig; fi
    ;;
  esac
  status=${status//$'\r'/}
  [ "$status" = ok ] || { echo "sign: Signatur von ${f##*/} nicht gültig ($status)" >&2; exit 1; }
  echo "sign: ${f##*/} signiert"
done
