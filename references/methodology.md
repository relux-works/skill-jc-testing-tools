# What each dev-cycle stage actually verifies (and doesn't)

A common mistake is treating an earlier stage passing as evidence a later stage will too. It isn't -- each stage exercises a genuinely different part of the stack, and this project hit a distinct, real bug at every single one of them.

| Stage | What it proves | What it does NOT prove |
|---|---|---|
| jCardSim test passes | Your applet's business logic is correct against a real JC API surface, on a real JVM | Whether the code is convertible to a real CAP at all (String exceptions, `System.arraycopy`, `int` array indexing all pass silently here -- see [codegen-jc-classic-compatibility.md](codegen-jc-classic-compatibility.md)) |
| CAP builds + verifies | The bytecode is within the convertible JC Classic subset for the *kit version you built against* | Whether that JC API level matches the physical card's actual runtime ceiling (see [cap-build-toolchain.md](cap-build-toolchain.md) Gotcha 1); whether the card supports optional capabilities like `ints="true"` declares |
| `LOAD`+`INSTALL` succeeds | The card accepted the package/applet at the GlobalPlatform level, and its JC runtime accepted the API level and capabilities the CAP declares | Whether the applet's runtime logic actually works when driven by real APDUs over the real transport (protocol/Le/response-chaining issues live at this layer, not the GP layer) |
| A raw APDU smoke test passes | The applet's `process()` dispatch and your APDU framing are correct for the exact commands you tried | Whether a *different* request shape (e.g. an explicit `Le` byte your test never used) works -- see the `Le` gotcha below |

## The explicit-`Le` gotcha

A request built as a "Case 2" APDU (no data, explicit `Le` requesting N response bytes) can reproducibly break the PC/SC transaction (`SCARD_E_NOT_TRANSACTED`) on some reader/card/applet combinations, while the *identical* request built as a bare "Case 1" APDU (no `Le` at all, letting the applet's own `setOutgoingAndSend()` determine the actual response length) works cleanly. This was isolated by testing the same command both ways and by testing it as the very first command in a fresh session (ruling out "N-th command in a session" as the cause). If a command mysteriously breaks the transaction while others in the same session work fine, check whether it's the one place you're constructing an explicit-`Le` request.

Generated `javacard-rpc` clients never request an explicit `Le` (their `APDUCommand` only appends one if you explicitly ask), so this specific gotcha does not carry forward if you're using the codegen pattern -- it mainly bites hand-rolled raw-APDU test harnesses.

## Card state after a failed/aborted transaction

A failed transaction (e.g. the `Le` gotcha above) can leave the card in a state where the *next* command on a fresh connection briefly returns an unexpected SW (e.g. `6E 00` on a `SELECT` that worked fine moments earlier). This has been observed to clear on its own with a simple fresh reconnect -- no physical reseat required. If you see a transient wrong-SW right after a failure, retry with a fresh connection before assuming something is actually broken.

## Recognizing "protocol/parameter problem" vs "logic/key problem" from the failure pattern alone

If **every** attempt fails identically regardless of which key/AID/command you send: that's a connection-parameter problem (see [t0-vs-t1.md](t0-vs-t1.md)), not a content problem. If failures vary in a way that correlates with *which* key/version/SCP/`i` you tried: that's a real key-discovery problem, and the safe method in [safe-gp-key-discovery.md](safe-gp-key-discovery.md) applies. Diagnosing which category you're in first saves a lot of wasted effort chasing the wrong hypothesis.
