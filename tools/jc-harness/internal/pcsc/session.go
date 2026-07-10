// Package pcsc wraps github.com/ebfe/scard with the two hard-won lessons from
// the bsim-javacard-helloworld / bsim-pcsc-bridge-go dev cycle:
//
//  1. Force protocol T=0 explicitly on connect. Letting the OS negotiate
//     ("any" protocol) reliably picks T=1 on readers/cards that formally
//     advertise both in their ATR but only actually work over T=0, and every
//     subsequent APDU then fails with SCARD_E_NOT_TRANSACTED.
//  2. ebfe/scard's raw Transmit does not auto-chase T=0 procedure bytes the
//     way javax.smartcardio does transparently. A response can come back as
//     a bare `61 xx` ("xx more bytes, call GET RESPONSE") or `6C xx` ("wrong
//     Le, retry with this one") instead of the actual data -- both must be
//     resolved here so callers always see the final result.
package pcsc

import (
	"fmt"
	"strings"

	"github.com/ebfe/scard"
)

// Session is a T=0-forced connection to a physical smart card reader.
type Session struct {
	ctx        *scard.Context
	card       *scard.Card
	ReaderName string
}

// maxT0Chain bounds the GET RESPONSE / 6Cxx retry chain, purely as a sanity
// guard against a misbehaving card looping forever.
const maxT0Chain = 8

// ListReaders returns all PC/SC reader names currently visible to the system.
func ListReaders() ([]string, error) {
	ctx, err := scard.EstablishContext()
	if err != nil {
		return nil, fmt.Errorf("establish PC/SC context: %w", err)
	}
	defer ctx.Release()
	return ctx.ListReaders()
}

// Connect opens a T=0-forced session against the first reader whose name
// contains nameFilter (case-insensitive). Pass "" to match the first reader.
func Connect(nameFilter string) (*Session, error) {
	ctx, err := scard.EstablishContext()
	if err != nil {
		return nil, fmt.Errorf("establish PC/SC context: %w", err)
	}

	readers, err := ctx.ListReaders()
	if err != nil {
		ctx.Release()
		return nil, fmt.Errorf("list readers: %w", err)
	}

	var readerName string
	for _, r := range readers {
		if nameFilter == "" || strings.Contains(strings.ToLower(r), strings.ToLower(nameFilter)) {
			readerName = r
			break
		}
	}
	if readerName == "" {
		ctx.Release()
		return nil, fmt.Errorf("no reader matching %q found (available: %v)", nameFilter, readers)
	}

	card, err := ctx.Connect(readerName, scard.ShareExclusive, scard.ProtocolT0)
	if err != nil {
		ctx.Release()
		return nil, fmt.Errorf("connect (T=0) to %q: %w", readerName, err)
	}

	return &Session{ctx: ctx, card: card, ReaderName: readerName}, nil
}

// Transmit sends a raw C-APDU and returns the fully-resolved R-APDU,
// transparently chasing any T=0 procedure bytes.
func (s *Session) Transmit(capdu []byte) ([]byte, error) {
	resp, err := s.card.Transmit(capdu)
	if err != nil {
		return nil, fmt.Errorf("transmit: %w", err)
	}
	return s.resolveT0Chaining(capdu, resp)
}

func (s *Session) resolveT0Chaining(lastCmd, resp []byte) ([]byte, error) {
	for i := 0; i < maxT0Chain; i++ {
		if len(resp) != 2 {
			return resp, nil
		}
		sw1, sw2 := resp[0], resp[1]
		switch sw1 {
		case 0x61:
			getResp := []byte{lastCmd[0], 0xC0, 0x00, 0x00, sw2}
			next, err := s.card.Transmit(getResp)
			if err != nil {
				return nil, fmt.Errorf("GET RESPONSE: %w", err)
			}
			lastCmd = getResp
			resp = next
		case 0x6C:
			retry := append([]byte(nil), lastCmd[:4]...)
			retry = append(retry, sw2)
			next, err := s.card.Transmit(retry)
			if err != nil {
				return nil, fmt.Errorf("retransmit with corrected Le: %w", err)
			}
			lastCmd = retry
			resp = next
		default:
			return resp, nil
		}
	}
	return nil, fmt.Errorf("T=0 procedure byte chain exceeded %d steps", maxT0Chain)
}

// Select sends a plain SELECT-by-AID APDU (classic dialect, CLA=0x00) with no
// explicit Le -- an explicit Le on some commands has been observed to
// reproducibly break the PC/SC transaction on some reader/card/applet
// combinations (see bsim-javacard-helloworld's harness notes); omitting it
// and letting the applet's own setOutgoingAndSend() determine the response
// length is the safe default this harness always uses.
func (s *Session) Select(aid []byte) ([]byte, error) {
	cmd := append([]byte{0x00, 0xA4, 0x04, 0x00, byte(len(aid))}, aid...)
	return s.Transmit(cmd)
}

// ATR returns the card's Answer-To-Reset bytes.
func (s *Session) ATR() ([]byte, error) {
	status, err := s.card.Status()
	if err != nil {
		return nil, fmt.Errorf("status: %w", err)
	}
	return status.Atr, nil
}

// Close releases the card handle and PC/SC context.
func (s *Session) Close() error {
	if s.card != nil {
		_ = s.card.Disconnect(scard.LeaveCard)
	}
	if s.ctx != nil {
		_ = s.ctx.Release()
	}
	return nil
}
