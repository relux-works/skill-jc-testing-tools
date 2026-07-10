# Finding the correct GP key/SCP/version/i without risking the card

You typically get a key set (KIC/ENC, KID/MAC, KIK/DEK) plus maybe a key version number from whoever issued the test card, but not necessarily the exact SCP protocol (SCP02 vs SCP03), the `i` parameter (SCP implementation-options byte), or which key version applies if the card exposes more than one key set. Guessing wrong at the `EXTERNAL AUTHENTICATE` step is a real, not-always-reversible action on some card configurations (repeated failures can lock the security domain). You do not have to guess blind.

## Why the discovery step is provably safe

`GPSession.openSecureChannel()` (GlobalPlatformPro's library API, used by `gp-t0-helper`'s `trysc` mode) does this internally:

1. Sends `INITIALIZE UPDATE` -- a plain, unauthenticated challenge/response. This never fails due to a wrong key (the key isn't evaluated yet) and never touches a retry counter.
2. Computes the expected card cryptogram **locally**, from the key/SCP/version/i you gave it, and compares it against what the card returned.
3. **Only if that local check matches** does it proceed to send `EXTERNAL AUTHENTICATE` (the step that actually touches the card's real security/retry state).

So: as long as you only call `trysc` (which stops at step 2) and never force past a mismatch, every wrong guess is free. A correct guess passing the local check and proceeding to a real `EXTERNAL AUTHENTICATE` is not a risk either -- a matching local cryptogram means the keys are correct, so the real authentication is expected (and, in practice, has always been observed) to succeed too.

## The method

1. Read the card's Key Info Template via a read-only `GPSession.discover()` + `getKeyInfoTemplate()` (no key required) -- this lists the key set(s) actually present on the card (type: AES or DES3, version number, id, length). This narrows the search space: e.g. an AES entry at version 1 suggests SCP03, a DES3 entry at version 0x20 suggests SCP02 -- but **treat this as a hint, not a guarantee**; it can be wrong (see below).
2. Enumerate combinations of {key pair you have} x {SCP02/SCP03} x {key version from the template} x {plausible `i` values} and run each through `trysc`. Common `i` values worth trying for a 3-static-key SCP02 setup: `0x15`, `0x55`, `0x05`, `0x0A`. For SCP03: `0x10`, `0x30`, `0x70`, `0x00` (varies by the number-of-base-keys/pseudo-random-challenge bits the card expects).
3. Stop at the first `trysc` that reports a passing local cryptogram check. That's your confirmed combination -- use it for the real `install`/`delete`.

## What "hint, not guarantee" means in practice

In the session this was distilled from, the Key Info Template showed both an AES (v1) and a DES3 (v0x20) entry. The naive assumption -- "the key pair labeled '1' pairs with the version-1 AES entry (SCP03), the pair labeled '2' pairs with the DES3 entry (SCP02)" -- was wrong. Every SCP03 attempt against the AES entry failed with `INITIALIZE UPDATE failed: 0x6A88` (referenced data not found) regardless of `i`, meaning that key version wasn't actually addressable as an SCP03 secure-channel key set the way the template's type label implied. The correct combination turned out to be the *other* key pair against the DES3/SCP02 entry. **Don't skip the enumeration step because the template "obviously" tells you which is which** -- confirm empirically.

## Two more failure signatures you'll see along the way (also harmless)

- `INITIALIZE UPDATE failed: 0x6700` (wrong length) -- a pure protocol-level rejection before any key material is evaluated. Usually means the `i` parameter implies a command shape (e.g. SCP03's "1 base key with KDF" mode vs "3 static keys" mode) that doesn't match what you're actually sending. Not a key problem; try a different `i`.
- Card cryptogram mismatch (the case this whole method exists to filter out safely) -- wrong key pair, wrong version, or wrong `i` for an otherwise-plausible SCP. Try the next combination.
