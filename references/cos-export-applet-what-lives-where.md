# What lives where: the COS library, the export files, and your applet

The single most common confusion when enabling a privileged capability
(`uicc.access` FileView, `uicc.toolkit` BIP/timers): mixing up three separate
things that live in three separate places. Sort these and the rest falls out.

## Three things, three places

```
┌─ SIM chip ─────────────────────────────────────┐
│  COS (firmware, ROM — vendor-baked, immutable):  │
│    • Java Card VM                                │
│    • GlobalPlatform                              │
│    • uicc.access / uicc.toolkit   ← THE LIBRARY  │  ← is it here? (vendor decides; cannot be added)
│                                                  │
│  EEPROM (you load here over GP):                 │
│    • YourApplet  (AID F0…)        ← YOU put this │  ← loaded by you, never pre-in the COS
└──────────────────────────────────────────────────┘

Your build machine (compile only):
    • uicc.access.exp / uicc.toolkit.exp   ← THE EXPORT FILES  (never go on the card)
```

1. **The library** (`uicc.access`, `uicc.toolkit`) is a **platform package in the
   COS firmware**. Either the vendor's COS build includes it or it does not; you
   **cannot add it** (it is ROM/native, not a loadable applet). This is the
   unknown you confirm with the vendor.
2. **The export files** (`.exp`) live **only on your build machine**. They are
   build-time linking metadata — the shape of the library so the converter can
   resolve your calls. They are **never loaded onto the card**, never in the CAP,
   never shipped with your applet. Think headers / an SDK stub.
3. **Your applet** (e.g. `HelloApplet`) is **not pre-in the COS**. **You load it
   yourself** over GlobalPlatform (`LOAD` + `INSTALL`, with your keys), into
   EEPROM. It runs as an application **on top of** the COS.

## The two linkings — both must succeed

- **Build-time link** (convert): your applet's calls resolve against the **export
  files** on your machine. No `.exp` -> it does not compile.
- **Install-time link** (`INSTALL` on the card): your applet's imported package
  AIDs resolve against **what is actually in the COS**. Package present ->
  resolves, applet installs and can call it. Package absent -> install fails with
  a linking error -> you need a **different card** whose COS ships it.

## Confusions this clears

- **"How do we get our applet into the COS?"** — there is no such task. You never
  put your applet in the COS. You load it into EEPROM over GP yourself; it is a
  tenant on top of the platform, not part of it.
- **"Are the export files in the COS?"** — no. They are on your build machine,
  compile-time only.
- **"What must already be in the COS, then?"** — only the **library**. That is the
  one thing you cannot supply and must confirm the vendor's card build has.

## The entitlement analogy (in iOS terms)

Export files are **not** the permission — they are the SDK you compile against.
The permission is a separate thing set at install:

| Ours | iOS analogue |
|---|---|
| library `uicc.access` / `uicc.toolkit` in the COS | the **framework being present** in the OS |
| export file `.exp` | the **SDK / headers** you compile against (build-time, not shipped) |
| Access Domain / BIP privilege (install parameters, set by the keys) | the **entitlement** — the permission to actually use the capability |

## So, to run a privileged applet you need all of

1. the **library present in the COS** (vendor card build), and
2. the **export files** to build the applet against (vendor sends; stay on your
   machine), and
3. the **privilege / Access Domain granted at install** — expressed in the
   install parameters, authorized by the keys (which for CARD#5 we already hold),
4. then **you load the applet** over GP yourself.

Items 3-4 we can already do with our keys. Items 1-2 are what a test card without
an issuer relationship is missing, and no key material substitutes for them —
see [applet-file-access-access-domain.md](applet-file-access-access-domain.md)
and [ios-sim-applet-bip-messaging.md](ios-sim-applet-bip-messaging.md).
