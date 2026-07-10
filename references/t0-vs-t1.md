# T=0 vs T=1: why it must be forced explicitly

Two APDU transport protocols exist at the ISO 7816-3 level, negotiated at `SCardConnect`:

- **T=0** -- byte-oriented, half-duplex: command, then a separate response; long responses need an explicit `GET RESPONSE` follow-up. The default (and often *only* reliably working) protocol on classic GSM SIMs and many test UICCs.
- **T=1** -- block-oriented: the whole APDU goes as one block with CRC/LRC. Common on USIM/UICC (3G/4G) and general-purpose smart cards.

A card's ATR (its interface bytes, TA/TB/TC/TD) declares which protocol(s) it supports. **The failure mode this reference exists for:** a card can formally advertise support for both T=0 and T=1 in its ATR while only actually working reliably over T=0. If your PC/SC client doesn't force a protocol and just asks for "any" (`SCARD_PROTOCOL_T0 | SCARD_PROTOCOL_T1`), the OS/CCID driver can -- and on real hardware, does -- negotiate T=1, and then **every single APDU fails** with `SCARD_E_NOT_TRANSACTED`, even though:

- the reader is detected fine,
- the ATR reads fine and is stable,
- the failure looks identical regardless of which key/AID/command you send, which makes it very easy to misdiagnose as a key or protocol-layer bug in your own code instead of a connection-parameter problem.

## The fix

Force the protocol explicitly at connect time. In `javax.smartcardio`: `terminal.connect("T=0")` (not `"*"`). In `github.com/ebfe/scard` (Go): `ctx.Connect(reader, scard.ShareExclusive, scard.ProtocolT0)` (not `scard.ProtocolAny`). This is what `jc-harness` and `gp-t0-helper` both do unconditionally -- there is no "auto" mode, because "auto" is exactly the thing that breaks.

## How to actually diagnose this if you hit it fresh

1. Confirm the reader/ATR are stable and readable (`jc-harness atr`) -- if this fails, the problem is earlier (USB/driver/no card), not protocol.
2. Try the most trivial possible APDU (`SELECT MF`, `00 A4 00 00 02 3F 00`) with the protocol forced to T=0 explicitly, bypassing whatever higher-level library/CLI tool you were using. If it now returns *any* SW (even an error SW like `6A 82`), that's success at this layer -- you got a real response, meaning T=0 forcing was the fix. If it still fails at the PC/SC level, the problem isn't protocol negotiation.
3. Only after that, reintroduce your actual keys/AIDs/commands.

**Don't waste time on other hypotheses first** (USB hub topology, bad cable, wrong key, wrong AID) if every single attempt fails identically regardless of what's being sent -- that pattern (uniform failure independent of content) is the signature of a connection-parameter problem, not a content problem. A hub/cable theory was tried and wrongly blamed once during the session this skill was distilled from, before the real T=0/T=1 cause was found by forcing the protocol explicitly and observing the difference directly.
