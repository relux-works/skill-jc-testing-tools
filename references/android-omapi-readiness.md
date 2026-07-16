# Android OMAPI readiness: slots, feature flags, and the two barriers

The last stage of the physical dev cycle is talking to your installed applet from
a real Android app over **OMAPI** (`android.se.omapi`) instead of the host PC/SC
reader. This stage has device- and card-dependent failure modes that are non-obvious
and cost real time to diagnose. This doc is the checklist; `scripts/android-omapi-probe.sh`
automates the diagnosis.

Distilled from a real bsim `phys-sim-dev-cycle` session (2026-07-15) validating a
demo app's `realDevice` flavor against a **Xiaomi M2101K6G (Redmi Note 10 Pro,
MIUI, Android 13 / API 33)** with a programmable test UICC.

## TL;DR readiness checklist

```bash
scripts/android-omapi-probe.sh                 # auto-pick the one physical device
scripts/android-omapi-probe.sh --serial <sn>   # or name it explicitly
scripts/android-omapi-probe.sh --serial <sn> --watch   # tail live OMAPI signals while your app runs
```

Static prerequisites the probe checks (no app needed):

1. `ro.build.version.sdk` >= 28 — OMAPI API exists since Android 9.
2. `android.hardware.se.omapi.uicc` feature present — **without it you cannot reach
   the SIM over OMAPI at all** (`.ese`/`.sd` are a different secure element).
3. Card is in the slot the firmware actually exposes (in practice: **slot 1**).

Then the on-card access-control layer must grant your app — which the probe's
`--watch` mode surfaces from logcat.

## Feature flags: OMAPI is not guaranteed, and not one thing

`android.se.omapi` the API exists on any API >= 28, but reachable secure elements
are declared per-kind by the vendor:

| feature | secure element |
| --- | --- |
| `android.hardware.se.omapi.uicc` | the SIM / UICC — what we want for applet access |
| `android.hardware.se.omapi.ese`  | embedded SE (typically NFC payments) |
| `android.hardware.se.omapi.sd`   | SD-card SE |

A device may declare **none, some, or all** of these. Only `.uicc` gives OMAPI
access to a SIM applet. A device with only `.ese` cannot reach the UICC over OMAPI
no matter what you do. Check with:

```bash
adb -s <sn> shell pm list features | grep -i omapi
```

## Slot selection: what's true vs the myth

**Myth:** "OMAPI doesn't let an app pick which SIM slot."

**Reality (GlobalPlatform Open Mobile API — Android Binding for OMAPI v3.3):**
selection exists and is per-slot **by reader name**. The app enumerates
`SEService.getReaders()` and picks the `Reader` whose `getName()` matches:

- a UICC reader for slot N is named `"SIM" + (SubscriptionInfo.getSimSlotIndex() + 1)`
  → `SIM1`, `SIM2`, … (the first slot is sometimes just `SIM`).
- `getReaders()` returns every available reader with no duplicates, even if no card
  is inserted.

**But** — and this is the load-bearing caveat — exposing a reader for a given
physical slot is **not mandatory**; it's up to the vendor SE HAL. The spec says
"*if* a second-slot reader is exposed it SHALL be named `SIM2`", not that one must
exist. **Many devices expose only `SIM1`**, regardless of which slot the card is in.
The observed Xiaomi exposes only `SIM1`; a card in slot 2 is simply unreachable over
OMAPI there. This is documented vendor behavior (see osmocom eUICC manual's Xiaomi /
HyperOS OMAPI caveats), not a bug in your app.

## How to check which slot the card is in

Two independent signals — one fast, one authoritative for OMAPI:

```bash
# Fast: which slot the MODEM sees the card in.
# Position in the comma list == physical slot number; LOADED = card read.
adb -s <sn> shell getprop gsm.sim.state
#   ABSENT,LOADED  -> slot1 empty, slot2 has card
#   LOADED,ABSENT  -> slot1 has card, slot2 empty
# Verified on the Xiaomi: moving the card slot2->slot1 flipped
# "ABSENT,LOADED" to "LOADED,ABSENT", confirming order == slot.

# Authoritative for OMAPI: what the SE service exposes and whether it sees a card.
# Only visible while an app calls SEService.getReaders()/isSecureElementPresent():
adb -s <sn> logcat -c        # clear first
# ...run your app/test...
adb -s <sn> logcat -d | grep -iE "getReader|isSecureElementPresent|AccessControl|ARF"
#   D SecureElementService: getReader() SIM1                 <- firmware exposes ONLY SIM1
#   I SecureElement-Terminal-SIM1: isSecureElementPresent()  <- true only if card in slot1
```

`gsm.sim.state` tells you where the card is *for the modem*; the logcat
`getReader()` / `isSecureElementPresent()` lines are the only trustworthy signal for
*OMAPI*. Trust the latter for OMAPI readiness. `--watch` mode of the probe tails
exactly these.

## The two barriers (observed, in order)

The realDevice OMAPI path clears layer by layer. Each layer has a distinct error.

### Barrier 1 — card not in the exposed slot

- `getReaders()` returns only `SIM1`; card is physically in slot 2.
- `SecureElement-Terminal-SIM1.isSecureElementPresent()` → `false`.
- App-visible error: **`No OMAPI reader with a secure element is available`**.
- Fix: **move the card to slot 1** (the only slot this firmware exposes).

### Barrier 2 — on-card access control (SEAC), no rules

With the card in slot 1, the reader is found, the SE is present, the session opens,
and a logical channel even opens (`SecureElement-Terminal-SIM1: the Channel id is 2`
— the card physically answered). Then Android's access-control enforcer denies:

```
E SecureElement-AccessControlEnforcer: No ARF exists
I SecureElement-AccessControlEnforcer: No ARF found in: SIM1
I SecureElement-AccessControlEnforcer: Deny any access to:SIM1
```

- App-visible error: **`AccessControlException: SecureElement-AccessControlEnforcer
  access denied: No ARF exists`**.
- This is **GlobalPlatform Secure Element Access Control (SEAC)**: before letting an
  app reach an AID, Android reads the card's access rules from one of:
  - an **ARA-M** applet (Access Rule Application Master) at AID `A00000015141434C00`,
    answering GET DATA rule queries; or
  - **ARF** — legacy PKCS#15 rule files under the UICC ADF.
- A card with **neither** → enforcer **fails closed** → no access to any AID.
- Fix (card-side provisioning, not app/SDK): install an **ARA-M applet with a rule**
  granting your app's signing-certificate hash access to your applet's AID (a dev
  "grant-all" rule is fine for a lab). The card is yours with GP keys, so it installs
  via the same GP-install loop as any applet (`gp-t0-helper`).

Barrier 2 is the point where "the app is done" and "the card is provisioned" diverge:
the same app/SDK code is green in a simulator/TCP-bridge flavor (no SEAC enforcer in
that path) and blocked here purely by on-card policy.

## MIUI / HyperOS install gotcha (not OMAPI, but same bring-up)

On MIUI/HyperOS, `connectedAndroidTest` (gradle UTP installer) fails with
`INSTALL_FAILED_USER_RESTRICTED: Install canceled by user` because the vendor shows
an on-device install confirmation the gradle runner won't wait for. Enable **both**
USB debugging *and* "Install via USB" in developer options; if it still trips,
bypass the gradle installer:

```bash
adb -s <sn> install -r -t app-realDevice-debug.apk
adb -s <sn> install -r -t app-realDevice-debug-androidTest.apk
adb -s <sn> shell am instrument -w \
  com.your.pkg.test/androidx.test.runner.AndroidJUnitRunner
```

## Primary sources

- GlobalPlatform, *Open Mobile API — Android Binding v1.0 for OMAPI v3.3* (reader
  naming `SIM`/`SIM2`, slot number = `getSimSlotIndex()+1`).
- AOSP `com.android.se` `SecureElementService` / `AccessControlEnforcer` (getReaders,
  isSecureElementPresent, ARA-M/ARF enforcement, fail-closed).
- osmocom eUICC manual, Android OMAPI device-specific caveats (vendor/RIL limitations,
  Xiaomi/HyperOS).
