# Why gp-t0-helper exists: GlobalPlatformPro's CLI can't force T=0

[GlobalPlatformPro](https://github.com/martinpaljak/GlobalPlatformPro) (`gp.jar`) is a mature, widely-used implementation of the GlobalPlatform secure-channel protocol (SCP02/SCP03: `INITIALIZE UPDATE`, `EXTERNAL AUTHENTICATE`, session-key derivation, MAC/ENC wrapping, `LOAD`/`INSTALL`/`DELETE`). Reimplementing that crypto to get T=0 forcing would be real, security-sensitive work duplicating an already-correct, already-reviewed implementation, for no actual benefit. `gp-t0-helper` does not do that. It works around one specific, narrow limitation instead.

## The limitation

`gp.jar`'s CLI has **no flag or environment variable to force the PC/SC protocol**. Confirmed by auditing its bytecode (`javap -c` on the extracted jar): it bundles its own JNA-based PC/SC binding (`jnasmartcardio`, not the JDK's built-in `sun.security.smartcardio`), and the class that actually opens the connection (`apdu4j.pcsc.CardTerminalAppRunner`) has a `protocol` field that always defaults to `"*"` unless overridden through an internal `AppParameters` map that the CLI never populates from any documented flag or the `GP_*` environment variables it does support (`GP_AID`, `GP_READER`, `GP_READER_IGNORE`, `GP_TRACE`, `GP_PCSC_RESET`, `GP_PCSC_EXCLUSIVE`, `GP_PCSC_TRANSACT` -- none of them touch protocol selection).

`"*"` means "let the OS pick," which is exactly the failure mode [t0-vs-t1.md](t0-vs-t1.md) describes.

## The workaround: use it as a library, not a CLI

`gp.jar`'s actual GlobalPlatform logic is exposed as a public library API (`pro.javacard.gp.GPSession`, `pro.javacard.gp.keys.PlaintextKeys`, etc.), separate from its CLI's connection-handling code. `gp-t0-helper` (`GpT0.java`) is a small driver that:

1. Connects via plain `javax.smartcardio.TerminalFactory` (the JDK's built-in provider -- the same PCSC.framework/pcsclite path, proven to work once T=0 is forced) and calls `terminal.connect("T=0")` explicitly.
2. Wraps the resulting `CardChannel` in a ~5-line `apdu4j.core.BIBO` implementation (`byte[] transceive(byte[] cmd)`).
3. Drives `GPSession.discover(bibo)`, `PlaintextKeys.fromKeys(...)`, `session.openSecureChannel(...)`, `session.loadCapFile(...)`, `session.installAndMakeSelectable(...)`, `session.deleteAID(...)` directly -- all real GlobalPlatformPro logic, unmodified. Only the connection step is replaced.

This is the general pattern for "a mature library's CLI can't do X, but the library itself can": find the public API boundary between "connection setup" and "actual protocol logic," and only replace the former.

## One more real gotcha hit along the way: `loadCapFile`'s second parameter

`GPSession.loadCapFile(CAPFile cap, AID sdAid, GPData.LFDBH hash)` -- the second `AID` parameter is the **target Security Domain AID** (pass `null` to mean "the currently-authenticated SD"), not the package AID. The package AID is read from the CAP file itself (`cap.getPackageAID()`). Passing your own package AID there (an easy mistake given the parameter's position right after the CAP file) makes the card look for a Security Domain that doesn't exist, and fails with `INSTALL [for load] failed: 0x6A88` (referenced data not found) -- a confusing error that looks like a permissions/key problem but is actually a wrong-parameter problem. Confirmed by disassembling `GPSession`'s bytecode, not from any published docs.
