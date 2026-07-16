import java.util.Arrays;

public class GpT0Test {
    public static void main(String[] args) {
        testHexParsing();
        testSecureApduValidation();
        System.out.println("GpT0Test PASS");
    }

    private static void testHexParsing() {
        assertArrayEquals(new byte[]{(byte) 0x80, (byte) 0xE2, (byte) 0x90, 0x00},
                GpT0.hex("80e29000"), "hex parsing");
        expectIllegalArgument("odd-length hex", "even number of characters",
                () -> GpT0.hex("ABC"));
    }

    private static void testSecureApduValidation() {
        String key = "00000000000000000000000000000000";
        GpT0.validateSecureApduArgs(new String[]{
                "secure-apdu", key, key, key, "20", "SCP02", "15", "80E62000"
        });

        expectIllegalArgument("missing APDU", "secure-apdu <kic>",
                () -> GpT0.validateSecureApduArgs(new String[]{
                        "secure-apdu", key, key, key, "20", "SCP02", "15"
                }));
        expectIllegalArgument("short APDU", "at least CLA/INS/P1/P2",
                () -> GpT0.validateSecureApduArgs(new String[]{
                        "secure-apdu", key, key, key, "20", "SCP02", "15", "80E2"
                }));
        expectIllegalArgument("unknown SCP", "No enum constant",
                () -> GpT0.validateSecureApduArgs(new String[]{
                        "secure-apdu", key, key, key, "20", "SCP99", "15", "80E62000"
                }));
    }

    private static void expectIllegalArgument(String label, String expectedMessage, Runnable action) {
        try {
            action.run();
            throw new AssertionError(label + " did not fail");
        } catch (IllegalArgumentException e) {
            if (e.getMessage() == null || !e.getMessage().contains(expectedMessage)) {
                throw new AssertionError(label + " returned unexpected message: " + e.getMessage());
            }
        }
    }

    private static void assertArrayEquals(byte[] expected, byte[] actual, String label) {
        if (!Arrays.equals(expected, actual)) {
            throw new AssertionError(label + " mismatch");
        }
    }
}
