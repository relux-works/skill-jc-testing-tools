#!/usr/bin/env bash
#
# android-omapi-probe.sh -- diagnose whether a connected Android device can
# reach a UICC applet over OMAPI (android.se.omapi), and if not, which known
# barrier you are hitting.
#
# Distilled from a real bsim phys-sim-dev-cycle session (2026-07-15) validating
# a demo app's realDevice flavor against a Xiaomi M2101K6G, which surfaced two
# sequential OMAPI barriers:
#   1. card in the wrong physical SIM slot -> getReaders() only exposes SIM1 on
#      this vendor firmware -> "No OMAPI reader with a secure element is available"
#   2. card in slot 1 but no on-card access rules -> AccessControlEnforcer:
#      "No ARF exists" -> "Deny any access" (needs an ARA-M / ARF rule on the card)
#
# See references/android-omapi-readiness.md for the full methodology.
#
# This is a HOST-side adb wrapper on purpose -- it is NOT part of the jc-harness
# Go binary, which is PC/SC-only and has no business shelling out to adb.
#
# Usage:
#   android-omapi-probe.sh [--serial <adb-serial>]        # static readiness checks
#   android-omapi-probe.sh [--serial <adb-serial>] --watch # tail OMAPI logcat signals
#
# Static checks need no app installed. --watch clears logcat and tails the SE /
# AccessControlEnforcer signals so you run your app and see which barrier fires.

set -euo pipefail

SERIAL=""
WATCH=0

usage() {
  sed -n '2,30p' "$0" | sed 's/^# \{0,1\}//'
  exit "${1:-0}"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --serial) SERIAL="${2:?--serial needs a value}"; shift 2 ;;
    --watch)  WATCH=1; shift ;;
    -h|--help) usage 0 ;;
    *) echo "unknown arg: $1" >&2; usage 1 ;;
  esac
done

# --- resolve adb -----------------------------------------------------------
if ! command -v adb >/dev/null 2>&1; then
  for cand in "${ANDROID_HOME:-}/platform-tools/adb" \
              "${ANDROID_SDK_ROOT:-}/platform-tools/adb" \
              "$HOME/Library/Android/sdk/platform-tools/adb" \
              "$HOME/Android/Sdk/platform-tools/adb"; do
    if [[ -n "$cand" && -x "$cand" ]]; then PATH="$(dirname "$cand"):$PATH"; break; fi
  done
fi
command -v adb >/dev/null 2>&1 || { echo "error: adb not found (set ANDROID_HOME or add platform-tools to PATH)" >&2; exit 1; }

# --- resolve device serial -------------------------------------------------
if [[ -z "$SERIAL" ]]; then
  # all connected devices in state 'device' (bash 3.2-safe: no mapfile)
  DEVS=()
  PHYS=()
  while IFS= read -r d; do
    [[ -z "$d" ]] && continue
    DEVS+=("$d")
    [[ "$d" == emulator-* ]] || PHYS+=("$d")
  done < <(adb devices | awk 'NR>1 && $2=="device" {print $1}')
  if [[ ${#PHYS[@]} -eq 1 ]]; then
    SERIAL="${PHYS[0]}"
  elif [[ ${#DEVS[@]} -eq 1 ]]; then
    SERIAL="${DEVS[0]}"
  else
    echo "error: could not auto-pick a device; pass --serial. Connected:" >&2
    adb devices -l >&2
    exit 1
  fi
fi

ADB=(adb -s "$SERIAL")
MODEL="$("${ADB[@]}" shell getprop ro.product.model 2>/dev/null | tr -d '\r')"
echo "== device: $SERIAL ($MODEL) =="

if [[ "$WATCH" -eq 1 ]]; then
  echo "== --watch: clearing logcat, tailing OMAPI / AccessControlEnforcer signals =="
  echo "   Run your OMAPI app/test now. Ctrl-C to stop. Legend:"
  echo "     getReader() SIMn            -> which reader(s) the firmware exposes"
  echo "     isSecureElementPresent      -> whether that reader sees a card"
  echo "     AccessControlEnforcer/ARF   -> on-card access-rule (SEAC) decision"
  "${ADB[@]}" logcat -c
  exec "${ADB[@]}" logcat \
    | grep --line-buffered -iE "omapi|SEService|SecureElement|AccessControl|ARF|ARA"
fi

# --- static readiness checks ----------------------------------------------
verdict_ok=1

# 1) Android SDK level (OMAPI android.se.omapi exists since API 28)
SDK="$("${ADB[@]}" shell getprop ro.build.version.sdk 2>/dev/null | tr -d '\r')"
REL="$("${ADB[@]}" shell getprop ro.build.version.release 2>/dev/null | tr -d '\r')"
if [[ "${SDK:-0}" -ge 28 ]]; then
  echo "[ok]   Android $REL (API $SDK) >= 28 (OMAPI API available)"
else
  echo "[FAIL] Android $REL (API $SDK) < 28 -- android.se.omapi not available"
  verdict_ok=0
fi

# 2) OMAPI feature flags
FEATS="$("${ADB[@]}" shell pm list features 2>/dev/null | tr -d '\r')"
has_feat() { grep -q "android.hardware.se.omapi.$1" <<<"$FEATS"; }
if has_feat uicc; then
  echo "[ok]   feature android.hardware.se.omapi.uicc present (SIM/UICC reachable as SE)"
else
  echo "[FAIL] feature android.hardware.se.omapi.uicc ABSENT -- cannot reach the SIM over OMAPI"
  verdict_ok=0
fi
has_feat ese && echo "[info] feature .ese present (embedded SE, usually NFC payments)"
has_feat sd  && echo "[info] feature .sd present (SD-card SE)"

# 3) SIM slot occupancy (position in gsm.sim.state == physical slot order)
STATE="$("${ADB[@]}" shell getprop gsm.sim.state 2>/dev/null | tr -d '\r')"
if [[ -n "$STATE" ]]; then
  echo "[info] gsm.sim.state = $STATE  (position N = physical slot N; LOADED = modem read the card)"
  slot=1
  loaded_slots=()
  IFS=',' read -ra PARTS <<<"$STATE"
  for p in "${PARTS[@]}"; do
    p_trim="$(echo "$p" | tr -d ' ')"
    echo "         slot $slot: $p_trim"
    [[ "$p_trim" == LOADED* || "$p_trim" == READY* ]] && loaded_slots+=("$slot")
    slot=$((slot+1))
  done
  if [[ ${#loaded_slots[@]} -gt 0 ]] && ! printf '%s\n' "${loaded_slots[@]}" | grep -qx 1; then
    echo "[WARN] card is in slot(s) ${loaded_slots[*]} but not slot 1."
    echo "       Most vendor firmwares expose ONLY the SIM1 OMAPI reader -- a card"
    echo "       in slot 2+ is typically unreachable over OMAPI. Move it to slot 1"
    echo "       and confirm with --watch (getReader() SIM1 / isSecureElementPresent)."
    verdict_ok=0
  fi
else
  echo "[info] gsm.sim.state unavailable (can't infer slot occupancy statically)"
fi

echo
if [[ "$verdict_ok" -eq 1 ]]; then
  echo "VERDICT: static OMAPI prerequisites look OK."
  echo "  Next: run your app/test, and use --watch to confirm the firmware exposes"
  echo "  a reader that sees the card, and that the on-card AccessControlEnforcer"
  echo "  grants access (no 'No ARF exists' / 'Deny any access'). If it denies, the"
  echo "  card needs ARA-M / ARF access rules for your app's signing-cert hash."
else
  echo "VERDICT: NOT ready -- fix the [FAIL]/[WARN] item(s) above before expecting OMAPI to work."
fi
