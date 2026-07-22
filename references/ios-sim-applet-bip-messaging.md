# iOS SIM-applet access: no on-device transport, and the BIP backend-mediated pattern

Why an iOS app cannot talk to an applet on the phone's UICC at all, and the
architecture that reaches the applet anyway: an async bidirectional messaging
fabric over BIP that bypasses iOS entirely.

## The wall

iOS exposes **no APDU path to the phone's own UICC / eSE** to third-party apps:
there is no OMAPI (that is Android), and no entitlement grants it. This is not a
"get the right entitlement" problem -- the capability does not exist. So there is
**no iOS `APDUTransport` to the SIM applet**, and no client-side transport can be
built for it.

(The one iOS APDU path is **CoreNFC** `NFCISO7816Tag.sendCommand` -- but that
talks to **external** contactless cards you tap, never the phone's own SIM. The
internal SWP/contact path is not exposed.)

## Reaching the applet from outside inverts the model

Since the app cannot reach in, the app becomes a plain HTTPS client to a
**backend hub**, and the applet is reached by one of two operator-mediated
channels:

| Direction | Mechanism | Initiator | Shape |
|---|---|---|---|
| **into the SIM** | OTA / **SMS-PP** secured packet (TS 102 225/226) | operator / backend | async, management-latency, operator OTA keys secure the payload |
| **out of the SIM** | **BIP** `OPEN CHANNEL` (CAT, TS 102 223) | the applet itself | async data session over the modem bearer |

OTA is operator-keyed and SMS-shaped -- fine for install/update/personalization,
absurd as a request/response channel. The interesting one for a product is BIP.

## The BIP async bidirectional fabric (the pattern for iOS)

The applet opens a data channel through the modem to **your** backend and runs
your own protocol over it. This link is **modem <-> SIM <-> network -- below iOS**;
Apple never sees it, so no OMAPI / entitlement is needed because the OS is not in
the path.

```
iOS app  ── HTTPS ──▶  your backend (hub)  ◀── BIP/TLS ──  UICC applet
                          |  correlates app <-> applet
                          |  queues commands, collects results
```

Mechanics:

- **Not a literal loop.** A Java Card applet is event-driven -- no thread, no
  `while(true)`. The "loop" is the STK **TIMER MANAGEMENT** mechanism: set a
  timer -> on expiry the toolkit framework re-invokes the applet -> it
  `OPEN CHANNEL`s to your backend, `SEND DATA` / `RECEIVE DATA`, `CLOSE`, re-arms
  the timer. Alternatively hold one channel open and long-poll. Latency = timer
  period / poll interval (seconds to tens of seconds).
- **Bidirectional within a session.** BIP has both `SEND DATA` (applet->server)
  and `RECEIVE DATA` (server->applet), so full duplex while the channel is open.
  You build your own async message bus (your framing, message types, queue) on
  top.
- **No spontaneous server->applet push while asleep.** The channel exists only
  while the applet holds it. Server->applet lands on the next wake / open. That
  is the async ceiling; seconds is fine, instant is not.
- **Trust is yours end-to-end.** The channel is TLS to your server with the
  applet's own keys. Unlike OTA -- where the operator's OTA keys secure the
  payload and the operator sits in the trust path -- with BIP + your crypto the
  operator is only in the **transport/privilege grant**, not the payload trust.

## Privilege model -- what is gated

Timers, BIP channels and the file Access Domain are all fields in the toolkit
applet's **install parameters** ("UICC Toolkit Application Specific Parameters",
TS 102 226), set at install by whoever holds the installing keys (the operator).

| Capability | Install-param field | Sensitivity |
|---|---|---|
| Timers | Maximum number of timers | trivial local resource -- effectively free, granted routinely |
| **BIP** | Maximum number of channels + network access | **guarded** -- off-device data; the operator weighs this |
| File read | Access Domain (+ DAP) | guarded -- EF_IMSI is privacy/regulatory |

There is **no zero-operator path**: being a toolkit applet on a subscriber's SIM
at all is an operator install. "Timer is free" buys nothing without BIP, and BIP
needs the guarded grant. On iPhone, BIP is a modem/baseband function below iOS
so it *may* work independent of Apple -- but reliability is
handset/baseband/carrier-dependent; treat it as "verify on the target
device+carrier", not a guaranteed yes.

## What our keys already do vs what to request (CARD#5 concrete)

With the CARD#5 ISD keys (KIC1/KID1/KIK1, SCP02) we can already
install/delete/update applets, do authenticated STORE DATA, and **put toolkit /
BIP / Access Domain values into the `INSTALL` parameters** -- i.e. the keys are
enough to *express* the grant. The block is upstream and is **not keys**:

- no **export files (`.exp`)** for `uicc.toolkit` / `uicc.access` / BIP to *build*
  an applet that calls timers / OPEN CHANNEL / FileView, and
- no confirmation the COS even implements those packages (all-`FF` CPLC, no docs).

Request from the card vendor / issuer (documentation + `.exp`, not more keys):

1. COS/platform identity and which ETSI packages it implements (`uicc.toolkit`,
   `uicc.access`, BIP/CAT bearer).
2. the **export files** for those packages at the card's COS version -- the
   concrete build blocker.
3. the install-parameter format the COS expects and the valid Access Domain /
   network-access values.
4. confirmation that the ISD keys carry the authority to grant network access /
   file access, or which privileged SD does.

See [applet-file-access-access-domain.md](applet-file-access-access-domain.md)
for the same three-gate structure applied to file reads; BIP is the same story
with "network access" in place of "file access".

## When this pattern is worth it

- **Plain identity (IMSI/ICCID): overkill.** The operator already knows them
  from network attach; a backend asks the operator's systems, not the SIM.
- **On-card secure operations: the real use.** A private key lives on the card
  and must sign/decrypt on demand; iOS cannot touch it. The app posts a challenge
  to the backend, the applet picks it up on its next BIP wake, signs on-card, and
  returns the result over BIP; the app collects it over HTTPS. SIM-as-secure-
  element, IoT SAFE-shaped -- the OS never in the trust path, everything through
  your backend hub over your own async bus.
