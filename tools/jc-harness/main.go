// jc-harness is a CLI harness for the physical JavaCard/UICC dev cycle,
// distilled from the bsim-javacard-helloworld / bsim-pcsc-bridge-go session.
//
// Reader/ATR/raw-APDU/smoke commands are native Go (github.com/ebfe/scard,
// T=0-forced). GlobalPlatform commands (secure-channel install/delete/key
// probing) shell out to the bundled Java helper in tools/gp-t0-helper --
// GlobalPlatform secure-channel crypto (SCP02/SCP03 session-key derivation,
// MAC/ENC) is deliberately NOT reimplemented here. GlobalPlatformPro is a
// mature, widely-used implementation of that crypto; duplicating it in Go
// would be real, security-sensitive work with no upstream review, for no
// benefit over shelling out. jc-harness's job is the part GlobalPlatformPro
// itself cannot do on this class of reader/card (forcing T=0), not
// reinventing GP.
//
// Agent-facing output: every command prints exactly one JSON object (or
// array) to stdout, success or failure -- never ad hoc human-readable text
// that has to be scraped. Errors print {"error": "..."} to stdout with a
// nonzero exit code, same shape whether the failure was a bad flag, a PC/SC
// fault, or a card-level SW. This project deliberately does NOT adopt the
// full agent-facing-api query-DSL (schema/filter/sort/pagination): jc-harness
// has a handful of imperative hardware actions, each returning one small
// fixed-shape result -- there is no multi-entity, multi-field dataset here
// for a projection/filtering layer to do useful work against. Plain JSON
// output is the part of that pattern's philosophy that actually applies.
//
// No flag has an implicit default: every value that affects which
// reader/card/AID/data is touched must be passed explicitly. A missing
// required flag is a hard error naming exactly what's missing and which
// command needs it -- never a silent fallback.
package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/relux-works/skill-jc-testing-tools/jc-harness/internal/pcsc"
)

func main() {
	if len(os.Args) < 2 {
		fail(fmt.Errorf("missing command (run \"jc-harness help\" for usage)"))
	}

	var result any
	var err error
	switch os.Args[1] {
	case "readers":
		result, err = cmdReaders()
	case "atr":
		result, err = cmdATR(os.Args[2:])
	case "apdu":
		result, err = cmdAPDU(os.Args[2:])
	case "select":
		result, err = cmdSelect(os.Args[2:])
	case "smoke":
		result, err = cmdSmoke(os.Args[2:])
	case "help", "-h", "--help":
		usage()
		return
	default:
		fail(fmt.Errorf("unknown command %q (run \"jc-harness help\" for usage)", os.Args[1]))
	}

	if err != nil {
		fail(err)
	}
	emit(result)
}

// emit prints one JSON value to stdout -- the only output contract this
// tool has. Human-readable help text is the one deliberate exception (see
// usage()), since it's invoked explicitly and never parsed by a caller.
func emit(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		// Encoding our own well-typed result structs should never fail; if it
		// does, that's a real bug worth a hard failure, not a silent partial write.
		fmt.Fprintf(os.Stderr, "internal error: encoding result: %v\n", err)
		os.Exit(1)
	}
}

func fail(err error) {
	emit(map[string]string{"error": err.Error()})
	os.Exit(1)
}

func usage() {
	fmt.Fprint(os.Stderr, `jc-harness -- physical JavaCard/UICC dev-cycle harness

Every command prints one JSON object/array to stdout (success or error --
{"error": "..."} on failure). No flag has a default value; every flag listed
below is required for that command.

Usage:
  jc-harness readers
      List PC/SC readers. No flags. -> {"readers": [...]}

  jc-harness atr --reader NAME
      Read the inserted card's ATR (T=0 forced). -> {"reader": ..., "atr": "hex"}

  jc-harness apdu --reader NAME --hex BYTES
      Send one raw APDU (no SELECT first). -> {"sw": "hex", "data": "hex"}

  jc-harness select --reader NAME --aid HEX
      SELECT by AID. -> {"sw": "hex", "data": "hex"}

  jc-harness smoke --reader NAME --aid HEX --apdu HEX[,HEX...]
      SELECT, then send each APDU in sequence.
      -> {"reader": ..., "select": {...}, "results": [{...}, ...]}

--reader takes a case-insensitive substring match against "jc-harness readers"
output (e.g. "OMNIKEY"), not necessarily the exact full name.

GlobalPlatform (install/delete/secure-channel key probing) is handled by the
bundled Java helper -- see tools/gp-t0-helper/README.md, not this binary.
`)
}

type readersResult struct {
	Readers []string `json:"readers"`
}

func cmdReaders() (any, error) {
	readers, err := pcsc.ListReaders()
	if err != nil {
		return nil, err
	}
	if readers == nil {
		readers = []string{}
	}
	return readersResult{Readers: readers}, nil
}

type atrResult struct {
	Reader string `json:"reader"`
	ATR    string `json:"atr"`
}

func cmdATR(args []string) (any, error) {
	reader, err := requireFlag(args, "--reader", "atr")
	if err != nil {
		return nil, err
	}
	sess, err := pcsc.Connect(reader)
	if err != nil {
		return nil, err
	}
	defer sess.Close()
	atr, err := sess.ATR()
	if err != nil {
		return nil, err
	}
	return atrResult{Reader: sess.ReaderName, ATR: hex.EncodeToString(atr)}, nil
}

// apduResult is the shape of every raw APDU/SELECT response: the trailing
// status word, split out from any response data (empty string, not omitted,
// when there is none -- a consistent shape beats an optional field here).
type apduResult struct {
	SW   string `json:"sw"`
	Data string `json:"data"`
}

func cmdAPDU(args []string) (any, error) {
	reader, err := requireFlag(args, "--reader", "apdu")
	if err != nil {
		return nil, err
	}
	apduHex, err := requireFlag(args, "--hex", "apdu")
	if err != nil {
		return nil, err
	}
	cmd, err := decodeHexFlag("--hex", apduHex)
	if err != nil {
		return nil, err
	}

	sess, err := pcsc.Connect(reader)
	if err != nil {
		return nil, err
	}
	defer sess.Close()

	resp, err := sess.Transmit(cmd)
	if err != nil {
		return nil, err
	}
	return toAPDUResult(resp)
}

func cmdSelect(args []string) (any, error) {
	reader, err := requireFlag(args, "--reader", "select")
	if err != nil {
		return nil, err
	}
	aidHex, err := requireFlag(args, "--aid", "select")
	if err != nil {
		return nil, err
	}
	aid, err := decodeHexFlag("--aid", aidHex)
	if err != nil {
		return nil, err
	}

	sess, err := pcsc.Connect(reader)
	if err != nil {
		return nil, err
	}
	defer sess.Close()

	resp, err := sess.Select(aid)
	if err != nil {
		return nil, err
	}
	return toAPDUResult(resp)
}

type smokeResult struct {
	Reader  string       `json:"reader"`
	Select  apduResult   `json:"select"`
	Results []apduResult `json:"results"`
}

func cmdSmoke(args []string) (any, error) {
	reader, err := requireFlag(args, "--reader", "smoke")
	if err != nil {
		return nil, err
	}
	aidHex, err := requireFlag(args, "--aid", "smoke")
	if err != nil {
		return nil, err
	}
	apdusArg, err := requireFlag(args, "--apdu", "smoke")
	if err != nil {
		return nil, err
	}
	aid, err := decodeHexFlag("--aid", aidHex)
	if err != nil {
		return nil, err
	}

	sess, err := pcsc.Connect(reader)
	if err != nil {
		return nil, err
	}
	defer sess.Close()

	selResp, err := sess.Select(aid)
	if err != nil {
		return nil, err
	}
	selResult, err := toAPDUResult(selResp)
	if err != nil {
		return nil, err
	}

	results := make([]apduResult, 0, len(strings.Split(apdusArg, ",")))
	for _, apduHex := range strings.Split(apdusArg, ",") {
		apduHex = strings.TrimSpace(apduHex)
		cmd, err := decodeHexFlag("--apdu", apduHex)
		if err != nil {
			return nil, err
		}
		resp, err := sess.Transmit(cmd)
		if err != nil {
			return nil, err
		}
		r, err := toAPDUResult(resp)
		if err != nil {
			return nil, err
		}
		results = append(results, r)
	}

	return smokeResult{Reader: sess.ReaderName, Select: selResult, Results: results}, nil
}

func toAPDUResult(resp []byte) (apduResult, error) {
	if len(resp) < 2 {
		return apduResult{}, fmt.Errorf("card response too short to contain a status word: %s", hex.EncodeToString(resp))
	}
	sw := resp[len(resp)-2:]
	data := resp[:len(resp)-2]
	return apduResult{SW: hex.EncodeToString(sw), Data: hex.EncodeToString(data)}, nil
}

// requireFlag returns the value following name in args, or a hard error
// naming the missing flag and the command it belongs to -- never a default.
func requireFlag(args []string, name, command string) (string, error) {
	for i, a := range args {
		if a == name {
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s: flag %s given but no value follows it", command, name)
			}
			return args[i+1], nil
		}
	}
	return "", fmt.Errorf("%s: missing required flag %s (run \"jc-harness help\" for usage)", command, name)
}

func decodeHexFlag(flagName, value string) ([]byte, error) {
	cleaned := strings.ReplaceAll(value, " ", "")
	decoded, err := hex.DecodeString(cleaned)
	if err != nil {
		return nil, fmt.Errorf("invalid %s value %q: not valid hex: %w", flagName, value, err)
	}
	if len(decoded) == 0 {
		return nil, fmt.Errorf("invalid %s value %q: decodes to zero bytes", flagName, value)
	}
	return decoded, nil
}
