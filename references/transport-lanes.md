# Transport lanes: sim → reader → device

The generated jcrpc client is transport-agnostic — it talks through an
`APDUTransport`, and you swap the implementation under it. The same client, the
same generated method calls, the same APDU contract run against three different
targets. Use them cheap-to-expensive so each lane catches its own class of bug
before you pay for the next.

| Lane | Transport | Where it runs | Target | Validates | Misses |
|---|---|---|---|---|---|
| **Simulator** | `TCPTransport` → bridge daemon | host JVM / Android emulator | jCardSim (software card) | applet + client logic, INS dispatch, RPC contract, response parsing | converter/runtime bugs, SD framing, OMAPI |
| **Reader** | [`OmnikeyTransport`](https://github.com/relux-works/jc-omnikey-transport) | host JVM only | real card in a PC/SC reader | real converter, real Security Domain framing, ATR, card quirks | OMAPI access control |
| **Device** | `OmapiTransport` | Android app on a phone | card in the phone's SIM slot | OMAPI access control (ARA-M grant), the full on-device stack | (this is the last gate) |

## Why the reader lane exists

`jCardSim` is a real JVM running your **applet class** — it is *not* a copy of
the card. It has no real GP keys, no classic file system, no ATR/COS, no OMAPI
enforcement. So "jCardSim green" is not "works on the card": the real converter
rejects things jCardSim tolerates (String exceptions, `int` array indexing,
`System.arraycopy`), and the real Security Domain frames STORE DATA differently
than a full-APDU simulator does. (Both classes of bug were caught only on
hardware — see [codegen-jc-classic-compatibility.md](codegen-jc-classic-compatibility.md)
and the bsim `processData` framing bug.)

The reader lane closes that gap **without a phone**: point the real client at
the real card in a reader, from a host JVM, right after applet development. You
reach the human-gated OMAPI phone lane with much more confidence, having already
proven the converter, the framing and the card's quirks.

It does **not** replace the device lane. A PC/SC reader talks to the card raw —
there is no OMAPI `AccessControlEnforcer` in the path, so ARA-M grants /
signing-cert access rules are only exercised on-device. Reader lane proves the
applet works; device lane proves the access-control layer around it works.

## The swap

Only the transport changes; the client and the generated method calls do not.

```kotlin
// Simulator: client → bridge → jCardSim
val sim = TCPTransport(host = "127.0.0.1", port = 9025).also { it.connect() }

// Reader: client → PC/SC (T=0 forced) → real card in the reader
val reader = OmnikeyTransport(readerName = "OMNIKEY").also { it.connect() }

// Device: client → OMAPI → card in the phone slot   (Android app only)
val device = OmapiTransport(/* ... */)

// same client, whichever transport:
val client = HelloAppletClient(HelloAppletBridgeTransport(reader))
client.select()
client.getIccid()
```

`OmnikeyTransport` is a raw APDU pipe over a stateful basic channel — the client
issues `SELECT` and every method call through `transmit`, exactly as against the
simulator bridge. It forces **T=0** at connect (see [t0-vs-t1.md](t0-vs-t1.md));
it is host-JVM only, since `javax.smartcardio` is not on Android.

## Recommended dev cycle with the lanes

1. Write the applet (+IDL) → **simulator lane** (jCardSim): iterate applet + client logic fast.
2. Build the real CAP, install on the card in the reader (jc-harness / gp-t0-helper).
3. **Reader lane** (`OmnikeyTransport`): run the real client against the real card — catch converter/framing/quirk bugs here, no phone, no card moves.
4. Flash the app to a phone, insert the card → **device lane** (`OmapiTransport`): validate OMAPI access control (ARA-M) and the full stack.

Steps 1 and 3 are host-side and fast; the human card-move is only the final
step 4.
