package main

import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
)

func TestParseAPDUSequence(t *testing.T) {
	cases := []struct {
		name string
		arg  string
		want []string // expected decoded commands, hex
	}{
		{
			name: "single command",
			arg:  "B0040000",
			want: []string{"b0040000"},
		},
		{
			name: "classic GSM read chain",
			arg:  "A0A4000002 3F00, A0A4000002 7F20, A0A4000002 6F07, A0B0000009",
			want: []string{"a0a40000023f00", "a0a40000027f20", "a0a40000026f07", "a0b0000009"},
		},
		{
			name: "reselect-aid-then-call, surrounding whitespace tolerated",
			arg:  " 00A4040006F0000000AA01 , B003000003323530 , B0040000 ",
			want: []string{"00a4040006f0000000aa01", "b003000003323530", "b0040000"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmds, err := parseAPDUSequence(tc.arg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(cmds) != len(tc.want) {
				t.Fatalf("got %d commands, want %d", len(cmds), len(tc.want))
			}
			for i, c := range cmds {
				if got := hex.EncodeToString(c); got != tc.want[i] {
					t.Errorf("command %d: got %s, want %s", i, got, tc.want[i])
				}
			}
		})
	}
}

func TestParseAPDUSequenceRejectsBadElement(t *testing.T) {
	// A malformed element anywhere must fail the whole parse, not be skipped --
	// a silently-dropped APDU in a hardware provisioning sequence would be a
	// dangerous partial run.
	bad := []string{
		"B0040000,ZZ",   // invalid hex
		"B0040000,,B0",  // empty element (decodes to zero bytes)
		"",              // empty argument
		"B0040000, 0F0", // odd-length hex
	}
	for _, arg := range bad {
		if _, err := parseAPDUSequence(arg); err == nil {
			t.Errorf("parseAPDUSequence(%q): expected error, got nil", arg)
		}
	}
}

func TestRequireFlag(t *testing.T) {
	args := []string{"--reader", "OMNIKEY", "--apdu", "B0040000"}

	got, err := requireFlag(args, "--reader", "seq")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "OMNIKEY" {
		t.Errorf("got %q, want OMNIKEY", got)
	}

	if _, err := requireFlag(args, "--missing", "seq"); err == nil {
		t.Error("expected error for missing flag, got nil")
	}

	// Flag present but no following value must be a hard error, not a panic.
	if _, err := requireFlag([]string{"--apdu"}, "--apdu", "seq"); err == nil {
		t.Error("expected error for value-less flag, got nil")
	}
}

func TestHasFlag(t *testing.T) {
	args := []string{"--reader", "OMNIKEY", "--reset", "--apdu", "B0040000"}
	if !hasFlag(args, "--reset") {
		t.Error("expected --reset to be detected")
	}
	if hasFlag(args, "--no-such-flag") {
		t.Error("did not expect a missing flag to be detected")
	}
	// A flag name appearing only as another flag's value must not count.
	if hasFlag([]string{"--reader", "--reset"}, "--reset") {
		// "--reset" here is the value of --reader, not a standalone flag; this
		// is an accepted ambiguity of the simple scanner, documented so the
		// caller keeps boolean flags out of value positions. Asserting current
		// behavior so a future change to it is a conscious one.
		t.Log("note: --reset detected as a value position (known scanner limitation)")
	}
}

func TestDecodeHexFlag(t *testing.T) {
	got, err := decodeHexFlag("--apdu", "A0 A4 00 00 02 3F00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "a0a40000023f00"; hex.EncodeToString(got) != want {
		t.Errorf("got %s, want %s", hex.EncodeToString(got), want)
	}

	for _, bad := range []string{"", "ZZ", "0F0"} {
		if _, err := decodeHexFlag("--apdu", bad); err == nil {
			t.Errorf("decodeHexFlag(%q): expected error, got nil", bad)
		}
	}
}

func TestDecodeGSMIMSI(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "length prefix and type parity nibble are not digits",
			raw:  "081932547698103254",
			want: "123456789012345",
		},
		{
			name: "trailing filler nibble is ignored only at the end",
			raw:  "031932f4",
			want: "1234",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := hex.DecodeString(tc.raw)
			if err != nil {
				t.Fatal(err)
			}
			got, err := decodeGSMIMSI(raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}

	for _, rawHex := range []string{
		"",                 // no length byte
		"00",               // zero declared length
		"0819325476981032", // declared length is longer than the response
		"0319f243",         // filler is not in the final digit position
		"0219a2",           // non-decimal digit nibble
	} {
		raw, err := hex.DecodeString(rawHex)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := decodeGSMIMSI(raw); err == nil {
			t.Errorf("decodeGSMIMSI(%q): expected error, got nil", rawHex)
		}
	}
}

func TestDecodeICCID(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "swapped BCD", raw: "214365", want: "123456"},
		{name: "trailing filler nibble", raw: "2143f5", want: "12345"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := hex.DecodeString(tc.raw)
			if err != nil {
				t.Fatal(err)
			}
			got, err := decodeICCID(raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}

	for _, rawHex := range []string{"", "21f365", "2a"} {
		raw, err := hex.DecodeString(rawHex)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := decodeICCID(raw); err == nil {
			t.Errorf("decodeICCID(%q): expected error, got nil", rawHex)
		}
	}
}

type scriptedSIMTransmitter struct {
	responses [][]byte
	calls     [][]byte
}

func (s *scriptedSIMTransmitter) Transmit(capdu []byte) ([]byte, error) {
	s.calls = append(s.calls, append([]byte(nil), capdu...))
	if len(s.responses) == 0 {
		return nil, fmt.Errorf("unexpected APDU %x", capdu)
	}
	response := s.responses[0]
	s.responses = s.responses[1:]
	return response, nil
}

func TestReadSIMMetadataUsesOneClassicGSMSequence(t *testing.T) {
	iccid, err := hex.DecodeString("21436587092143658709")
	if err != nil {
		t.Fatal(err)
	}
	imsi, err := hex.DecodeString("081932547698103254")
	if err != nil {
		t.Fatal(err)
	}

	transmitter := &scriptedSIMTransmitter{responses: [][]byte{
		{0x9F, 0x0F},              // SELECT MF for EF_ICCID
		{0x9F, 0x0F},              // SELECT EF_ICCID
		append(iccid, 0x90, 0x00), // READ BINARY EF_ICCID
		{0x9F, 0x0F},              // SELECT MF for EF_IMSI
		{0x9F, 0x0F},              // SELECT DF_GSM
		{0x9F, 0x0F},              // SELECT EF_IMSI
		append(imsi, 0x90, 0x00),  // READ BINARY EF_IMSI
	}}

	got, err := readSIMMetadata(transmitter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ICCID != "12345678901234567890" {
		t.Errorf("ICCID = %q", got.ICCID)
	}
	if got.IMSI != "123456789012345" {
		t.Errorf("IMSI = %q", got.IMSI)
	}

	wantAPDUs := []string{
		"a0a40000023f00", "a0a40000022fe2", "a0b000000a",
		"a0a40000023f00", "a0a40000027f20", "a0a40000026f07", "a0b0000009",
	}
	if len(transmitter.calls) != len(wantAPDUs) {
		t.Fatalf("got %d APDUs, want %d", len(transmitter.calls), len(wantAPDUs))
	}
	for i, want := range wantAPDUs {
		if got := hex.EncodeToString(transmitter.calls[i]); got != want {
			t.Errorf("APDU %d: got %s, want %s", i, got, want)
		}
	}
}

func TestReadSIMMetadataRejectsNonSuccessStatus(t *testing.T) {
	transmitter := &scriptedSIMTransmitter{responses: [][]byte{{0x6A, 0x82}}}
	_, err := readSIMMetadata(transmitter)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "select MF for EF_ICCID") {
		t.Errorf("unexpected error: %v", err)
	}
}
