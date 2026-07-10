import javax.smartcardio.*;
import apdu4j.core.*;
import pro.javacard.gp.*;
import pro.javacard.gp.keys.PlaintextKeys;
import pro.javacard.capfile.*;

import java.io.PrintStream;
import java.nio.file.Paths;
import java.util.EnumSet;

/**
 * Minimal GlobalPlatformPro driver that forces PC/SC protocol T=0 explicitly
 * (via javax.smartcardio CardTerminal.connect("T=0")), bypassing GPTool's CLI
 * path which always requests "*" and lets macOS negotiate T=1 on this
 * reader/card, which reliably fails SCardTransmit.
 *
 * Reuses GlobalPlatformPro's own GPSession/GPCommands/PlaintextKeys classes
 * (from gp.jar on the classpath) for all SCP crypto — this is not a
 * reimplementation of GlobalPlatform, just a different connection path into
 * the same library.
 *
 * Modes:
 *   list                                    -- read-only discover + registry dump
 *   install <cap> <pkgAid> <appletAid> <instanceAid> <kic> <kid> <kik>
 *                                            -- open secure channel, LOAD cap, INSTALL applet
 *   apdu <cla> <ins> <p1> <p2> [dataHex]     -- raw APDU after plain SELECT (no secure channel)
 */
public class GpT0 {

    public static void main(String[] args) throws Exception {
        if (args.length < 1) {
            System.err.println("usage: list | install <cap> <pkgAid> <appletAid> <instanceAid> <kic> <kid> <kik> | apdu <cla> <ins> <p1> <p2> [dataHex]");
            System.exit(2);
        }

        CardTerminal terminal = findReader();
        Card card = terminal.connect("T=0");
        System.out.println("Connected, protocol=" + card.getProtocol());
        CardChannel channel = card.getBasicChannel();

        BIBO bibo = new BIBO() {
            @Override
            public byte[] transceive(byte[] cmd) {
                try {
                    return channel.transmit(new javax.smartcardio.CommandAPDU(cmd)).getBytes();
                } catch (CardException e) {
                    throw new BIBOException("transmit failed", e);
                }
            }

            @Override
            public void close() {
            }
        };
        APDUBIBO apduBibo = new APDUBIBO(bibo);

        try {
            switch (args[0]) {
                case "list":
                    doList(apduBibo);
                    break;
                case "info":
                    GPData.dump(apduBibo);
                    break;
                case "trysc":
                    doTrySc(apduBibo, args);
                    break;
                case "install":
                    doInstall(apduBibo, args);
                    break;
                case "apdu":
                    doApdu(apduBibo, args);
                    break;
                case "smoke":
                    doSmoke(apduBibo, args);
                    break;
                case "delete":
                    doDelete(apduBibo, args);
                    break;
                default:
                    System.err.println("unknown mode: " + args[0]);
                    System.exit(2);
            }
        } finally {
            card.disconnect(true);
        }
    }

    private static CardTerminal findReader() throws CardException {
        TerminalFactory tf = TerminalFactory.getDefault();
        for (CardTerminal t : tf.terminals().list()) {
            if (t.getName().toLowerCase().contains("omnikey")) {
                return t;
            }
        }
        throw new IllegalStateException("OMNIKEY reader not found");
    }

    private static void doList(APDUBIBO bibo) throws Exception {
        GPSession session = GPSession.discover(bibo);
        System.out.println("SD AID: " + session.getAID());
        System.out.println("SCP: " + session.getSecureChannel());
        System.out.println("Key info template:");
        for (GPKeyInfo ki : session.getKeyInfoTemplate()) {
            System.out.println("  " + ki);
        }
        System.out.println("Registry:");
        GPCommands.listRegistry(session.getRegistry(), (PrintStream) System.out, true);
    }

    private static void doInstall(APDUBIBO bibo, String[] args) throws Exception {
        if (args.length < 8) {
            System.err.println("install <cap> <pkgAid> <appletAid> <instanceAid> <kic> <kid> <kik>");
            System.exit(2);
        }
        String capPath = args[1];
        AID pkgAid = AID.fromString(args[2]);
        AID appletAid = AID.fromString(args[3]);
        AID instanceAid = AID.fromString(args[4]);
        byte[] kic = hex(args[5]);
        byte[] kid = hex(args[6]);
        byte[] kik = hex(args[7]);
        int keyVersion = args.length > 8 ? Integer.parseInt(args[8], 16) : 1;
        String scpName = args.length > 9 ? args[9] : "SCP03";
        int scpI = args.length > 10 ? Integer.parseInt(args[10], 16) : 1;

        GPSession session = GPSession.discover(bibo);
        System.out.println("SD AID: " + session.getAID());

        PlaintextKeys keys = PlaintextKeys.fromKeys(kic, kid, kik);
        keys.setVersion(keyVersion);
        GPSecureChannelVersion scpVersion = new GPSecureChannelVersion(
                GPSecureChannelVersion.SCP.valueOf(scpName), scpI);
        System.out.println("Attempting secure channel: " + scpVersion + " keyVersion=0x" + Integer.toHexString(keyVersion) + " with provided keys...");
        session.openSecureChannel(keys, scpVersion, null, GPSession.defaultMode);
        System.out.println("Secure channel opened.");

        CAPFile cap = CAPFile.fromFile(Paths.get(capPath));
        // 2nd arg is the target Security Domain AID (null = currently selected/authenticated SD),
        // NOT the package AID -- the package AID is read from the CAP file itself.
        session.loadCapFile(cap, null, GPData.LFDBH.SHA256);
        System.out.println("CAP loaded.");

        session.installAndMakeSelectable(pkgAid, appletAid, instanceAid,
                EnumSet.noneOf(GPRegistryEntry.Privilege.class), new byte[0]);
        System.out.println("Applet installed and made selectable: " + instanceAid);
    }

    /**
     * Safe secure-channel-only probe: INITIALIZE UPDATE + local card-cryptogram
     * check, no EXTERNAL AUTHENTICATE, no LOAD/INSTALL. GPSession.openSecureChannel
     * throws before ever sending EXTERNAL AUTHENTICATE if the locally-derived
     * session keys don't reproduce the card's cryptogram, so a wrong guess here
     * never touches the card's real security/retry counters.
     */
    private static void doTrySc(APDUBIBO bibo, String[] args) throws Exception {
        if (args.length < 7) {
            System.err.println("trysc <kic> <kid> <kik> <keyVersionHex> <scpName> <iHex>");
            System.exit(2);
        }
        byte[] kic = hex(args[1]);
        byte[] kid = hex(args[2]);
        byte[] kik = hex(args[3]);
        int keyVersion = Integer.parseInt(args[4], 16);
        String scpName = args[5];
        int scpI = Integer.parseInt(args[6], 16);

        GPSession session = GPSession.discover(bibo);
        PlaintextKeys keys = PlaintextKeys.fromKeys(kic, kid, kik);
        keys.setVersion(keyVersion);
        GPSecureChannelVersion scpVersion = new GPSecureChannelVersion(
                GPSecureChannelVersion.SCP.valueOf(scpName), scpI);
        try {
            session.openSecureChannel(keys, scpVersion, null, GPSession.defaultMode);
            System.out.println("RESULT=PASS " + scpVersion + " keyVersion=0x" + Integer.toHexString(keyVersion));
        } catch (GPException e) {
            System.out.println("RESULT=FAIL " + scpVersion + " keyVersion=0x" + Integer.toHexString(keyVersion) + " reason=" + e.getMessage());
        }
    }

    /** SELECT the installed applet by AID, then exercise ECHO and GET_VERSION in the same session. */
    private static void doSmoke(APDUBIBO bibo, String[] args) throws Exception {
        if (args.length < 2) {
            System.err.println("smoke <appletAidHex>");
            System.exit(2);
        }
        byte[] aid = hex(args[1]);

        apdu4j.core.CommandAPDU select = new apdu4j.core.CommandAPDU(0x00, 0xA4, 0x04, 0x00, aid);
        apdu4j.core.ResponseAPDU r1 = bibo.transmit(select);
        System.out.println("SELECT " + args[1] + " -> SW=" + Integer.toHexString(r1.getSW()) + " data=" + toHex(r1.getData()));

        byte[] payload = hex("DEADBEEF");
        apdu4j.core.CommandAPDU echo = new apdu4j.core.CommandAPDU(0xB0, 0x01, 0x00, 0x00, payload);
        apdu4j.core.ResponseAPDU r2 = bibo.transmit(echo);
        System.out.println("ECHO DEADBEEF -> SW=" + Integer.toHexString(r2.getSW()) + " data=" + toHex(r2.getData()));

        // No explicit Le -- an explicit Le=2 here reproducibly breaks the PC/SC
        // transaction on this reader/card/applet combo (SCARD_E_NOT_TRANSACTED),
        // while the same request without Le works and returns the same 2 bytes.
        apdu4j.core.CommandAPDU getVer = new apdu4j.core.CommandAPDU(0xB0, 0x02, 0x00, 0x00);
        apdu4j.core.ResponseAPDU r3 = bibo.transmit(getVer);
        System.out.println("GET_VERSION -> SW=" + Integer.toHexString(r3.getSW()) + " data=" + toHex(r3.getData()));
    }

    /** Open secure channel and delete an AID (package + any instances), ignoring not-found. */
    private static void doDelete(APDUBIBO bibo, String[] args) throws Exception {
        if (args.length < 8) {
            System.err.println("delete <aidHex> <kic> <kid> <kik> <keyVersionHex> <scpName> <iHex>");
            System.exit(2);
        }
        AID aid = AID.fromString(args[1]);
        byte[] kic = hex(args[2]);
        byte[] kid = hex(args[3]);
        byte[] kik = hex(args[4]);
        int keyVersion = Integer.parseInt(args[5], 16);
        String scpName = args[6];
        int scpI = Integer.parseInt(args[7], 16);

        GPSession session = GPSession.discover(bibo);
        PlaintextKeys keys = PlaintextKeys.fromKeys(kic, kid, kik);
        keys.setVersion(keyVersion);
        session.openSecureChannel(keys, new GPSecureChannelVersion(GPSecureChannelVersion.SCP.valueOf(scpName), scpI), null, GPSession.defaultMode);
        try {
            session.deleteAID(aid, true);
            System.out.println("Deleted " + aid);
        } catch (GPException e) {
            System.out.println("Delete failed (may simply not exist yet): " + e.getMessage());
        }
    }

    private static void doApdu(APDUBIBO bibo, String[] args) throws Exception {
        if (args.length < 5) {
            System.err.println("apdu <cla> <ins> <p1> <p2> [dataHex]");
            System.exit(2);
        }
        int cla = Integer.parseInt(args[1], 16);
        int ins = Integer.parseInt(args[2], 16);
        int p1 = Integer.parseInt(args[3], 16);
        int p2 = Integer.parseInt(args[4], 16);
        byte[] data = args.length > 5 ? hex(args[5]) : new byte[0];
        apdu4j.core.CommandAPDU cmd = data.length > 0
                ? new apdu4j.core.CommandAPDU(cla, ins, p1, p2, data)
                : new apdu4j.core.CommandAPDU(cla, ins, p1, p2);
        apdu4j.core.ResponseAPDU resp = bibo.transmit(cmd);
        System.out.println("SW=" + Integer.toHexString(resp.getSW()) + " data=" + toHex(resp.getData()));
    }

    private static byte[] hex(String s) {
        int n = s.length() / 2;
        byte[] b = new byte[n];
        for (int i = 0; i < n; i++) {
            b[i] = (byte) Integer.parseInt(s.substring(i * 2, i * 2 + 2), 16);
        }
        return b;
    }

    private static String toHex(byte[] b) {
        StringBuilder sb = new StringBuilder();
        for (byte x : b) {
            sb.append(String.format("%02X ", x));
        }
        return sb.toString();
    }
}
