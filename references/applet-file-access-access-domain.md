# Applet file access: FileView, Access Domain and the issuer grant

How an applet reads the card's own files (EF_IMSI, EF_ICCID, ...) natively, why
that needs an issuer grant, and what the grant physically is. This is "Path A"
(read the live file in-applet) versus "Path B" (host reads the file over the
reader and provisions a copy into the applet).

## The problem

Base Java Card firewall: an applet cannot read another application's file
objects. UICC platforms are the qualified exception -- ETSI **TS 102 241**
defines `uicc.access.FileView`, which lets an applet `SELECT` / `READ BINARY` /
`READ RECORD` / `UPDATE` MF descendants (DF Telecom, DF GSM, EFs such as
`EF_IMSI 6F07`, `EF_ICCID 2FE2`). Each view has its own file context, so an
applet using it does not collide with the terminal's selected applet/file.

That is the live, authoritative, profile-swap-proof way to serve identity: the
MNO updates the EF, the read reflects it, no provisioning. But it only works if
two things both hold.

## Two prerequisites, both required

1. **The COS actually implements the `uicc.access` package.** This is a
   card-build property, not something granted per applet. If the platform does
   not expose the API, there is no in-applet file read at all. On an
   unidentified test UICC (all-`FF` CPLC, no vendor docs) treat it as unknown /
   unavailable until proven.
2. **The applet is granted an Access Domain** permitting those files. This is
   the issuer grant, and it is the part people mean by "issuer gives my applet
   rights".

## The grant is an Access Domain, not a key

Two different things are easy to conflate:

- **Secure-channel / SD keys** (KIC/KID/KIK, or a Security Domain's keys)
  authenticate **who** performs the install -- they prove you are allowed to
  install on this card. They open the GlobalPlatform secure channel. The issuer
  holds them, or delegates a Security Domain that carries its own keys.
- **Access Domain** (ETSI **TS 102 226**, in the "UICC System Specific
  Parameters" carried inside the GP `INSTALL [for install]` application-specific
  parameters) declares **what** the applet may access. It is a permission
  descriptor, not crypto: e.g. `00` = full access (still subject to the file's
  native ADM conditions), `FF` = no access, plus an Access Domain DAP for
  granular rules. The COS checks it against each file's native access conditions
  on **every** FileView operation (TS 102 241 requires this).

So: **keys = "I am allowed to grant", Access Domain = "the grant itself".**
Install the same applet without the right keys and the card either refuses the
install or refuses to set a privileged Access Domain.

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

- **No grant (or unconfirmed `uicc.access`) -> Path B.** The host reads the real
  EF over the reader (classic file access, `jc-harness`: `SELECT MF -> [DF_GSM
  ->] EF -> READ BINARY`) and injects a copy into the applet -- via a runtime
  setter APDU or an install parameter. It is a snapshot: it goes stale on a
  profile swap, and its trust equals whoever provisioned it. Setter vs install
  param is throwaway demo scaffolding; the only real difference is that an
  install parameter has no unauthenticated runtime write surface and rides the
  GP secure channel for free.
- **Grant + FileView -> Path A.** The applet reads the live EF directly. No
  provisioning, profile-swap-proof by construction, and the whole set-vs-install
  provisioning question evaporates.

## On a test UICC with no issuer relationship

Path A is untestable until the vendor / issuer either (a) confirms `uicc.access`
is present and installs your applet with an Access Domain that grants the EF
read, or (b) hands over ADM / SD keys plus the API docs so you can install with
the Access Domain yourself. Until then Path B is the only reality, and its
provisioning mechanism is scaffolding that the FileView read replaces once the
grant lands.
