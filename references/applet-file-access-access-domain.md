# Applet file access: FileView, Access Domain and the issuer grant

How an applet reads the card's own files (EF_IMSI, EF_ICCID, ...) natively, why
that needs an issuer grant, and -- importantly -- why having the install keys is
**not** what unblocks it. This is "Path A" (read the live file in-applet) versus
"Path B" (host reads the file over the reader and provisions a copy).

## The problem

Base Java Card firewall: an applet cannot read another application's file
objects. UICC platforms are the qualified exception -- ETSI **TS 102 241**
defines `uicc.access.FileView`, which lets an applet `SELECT` / `READ BINARY` /
`READ RECORD` / `UPDATE` MF descendants (DF Telecom, DF GSM, EFs such as
`EF_IMSI 6F07`, `EF_ICCID 2FE2`). Each view has its own file context, so an
applet using it does not collide with the terminal's selected applet/file.

That is the live, authoritative, profile-swap-proof way to serve identity: the
MNO updates the EF, the read reflects it, no provisioning.

## Three gates, in order -- and keys only open the last one

The tempting mental model is "we have the install keys, so we just pass the
applet a descriptor that lets it read EF_IMSI/EF_ICCID." That is backwards about
where the block is. Path A has **three** gates, and they fail in this order:

1. **Build gate -- export files.** To write an applet that calls
   `uicc.access.FileView`, the Java Card converter needs the `uicc.access`
   **export files (`.exp`)** for the card's COS version, to link against. Without
   them the applet does not convert. Keys do not give you export files.
2. **Install gate -- the package must exist in the COS.** `uicc.access` is a
   card-build feature. An applet that imports the package will not install on a
   card whose OS does not ship it -- the imported package AID does not resolve
   and the install fails with a linking error. Keys do not add an OS package
   that is not there. (On an unidentified test UICC -- all-`FF` CPLC, no vendor
   docs -- you cannot even confirm the package is present.)
3. **Grant gate -- the Access Domain.** Only now does the descriptor matter: the
   applet is installed with an **Access Domain** in its install parameters that
   permits the files. **This is the step the keys enable** -- and it is the last
   and easiest one.

So the block is at gates 1-2 (does the API exist on this card, and can we even
compile against it), **not** at gate 3. Passing the Access Domain descriptor was
never the hard part; there is just no API on the card to grant access through,
and nothing to build the applet from.

Analogy: you have **admin rights** (keys) to install a program that calls a
system library, but the OS does not ship that library (gate 2) and you do not
have its headers to compile your program (gate 1). Admin rights conjure neither.
The Access Domain is "permission to use the library" -- meaningless when there
is no library and no way to build against it.

## The grant is an Access Domain, not a key

Two things people conflate:

- **Secure-channel / SD keys** (KIC/KID/KIK, or a Security Domain's keys)
  authenticate **who** performs the install. They open the GlobalPlatform secure
  channel and prove you may install. The issuer holds them, or delegates a
  Security Domain that carries its own keys.
- **Access Domain** (ETSI **TS 102 226**, in the "UICC System Specific
  Parameters" inside the GP `INSTALL [for install]` application-specific
  parameters) declares **what** the applet may access. It is a permission
  descriptor, not crypto: `00` = full access (still subject to the file's native
  ADM conditions), `FF` = no access, plus an Access Domain DAP for granular
  rules. The COS checks it against each file's native access conditions on
  **every** FileView operation (TS 102 241 requires this).

**keys = "I am allowed to grant", Access Domain = "the grant itself".**

```
GP INSTALL [for install]
  |- wrapped in a secure channel on issuer / delegated-SD KEYS   <- proves "I may grant"
  '- Access Domain in the install parameters (e.g. 00)           <- the privilege itself
```

## Who installs it, and how

The file system and the Security Domain that may install privileged UICC
applets belong to the **issuer** (the MNO or its personalization vendor). A
third-party developer does not set their own Access Domain. Concretely, "the
issuer grants me rights" means the issuer installs (or authorizes installation
of) your CAP **with the Access Domain parameter set**:

- at **factory personalization**, or
- **OTA**: the operator sends a binary SMS (SMS-PP / BIP) carrying the GP
  `INSTALL`, signed and encrypted with OTA keys (KIC/KID); the card's ISD / RFM
  verifies and installs it. You never touch the operator's keys.

Two operational shapes:

- **Operator does it all.** You hand over the CAP plus the files and access
  level the applet needs; their perso / OTA system installs it with the Access
  Domain.
- **Delegated Security Domain (TSM / GP confidential card content management).**
  The operator provisions you a Security Domain with its own keys and a bounded
  access-domain scope; you manage your applets within that envelope, but the
  file access they can receive is still capped by what the issuer configured.

## The real friction

Mechanically possible, operationally guarded. `EF_IMSI` is a subscriber
identifier -- a privacy / regulatory concern (tracking, IMSI-catcher exposure),
so operators frequently refuse a third-party grant or gate it behind contracts /
GSMA compliance. `EF_ICCID` is less sensitive but still identifying. This is why
many "SIM applet reads identity" designs never obtain the grant and fall back to
Path B.

## Where this leaves the dev cycle

- **Gates 1-2 unmet (or unconfirmed) -> Path B.** The host reads the real EF over
  the reader (classic file access, `jc-harness`: `SELECT MF -> [DF_GSM ->] EF ->
  READ BINARY`) and injects a copy into the applet -- via a runtime setter APDU
  or an install parameter. It is a snapshot: it goes stale on a profile swap, and
  its trust equals whoever provisioned it. Setter vs install param is throwaway
  demo scaffolding; the only real difference is that an install parameter has no
  unauthenticated runtime write surface and rides the GP secure channel for free.
- **All three gates met -> Path A.** The applet reads the live EF directly. No
  provisioning, profile-swap-proof by construction, and the whole
  set-vs-install provisioning question evaporates.

## On a test UICC with no issuer relationship

Path A is untestable until the vendor / issuer provides, in order:

1. confirmation that the COS ships `uicc.access`,
2. the **export files** for that package/COS version so you can build the applet,
3. then your **keys** install it with an Access Domain that grants the EF read.

Item 3 is the part install keys already cover; items 1 and 2 are the ones you
are missing on a test card, and no amount of key material substitutes for them.
Until they land, Path B is the only reality, and its provisioning mechanism is
scaffolding that the FileView read replaces once the gates open.
