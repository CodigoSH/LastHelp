package webauthn

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// A CBOR reader (RFC 8949) for the little an authenticator sends: maps, arrays,
// integers, byte and text strings, and the simple values. It is not a general
// decoder and is not meant to be — what an authenticator writes is small, and a
// reader that only knows those shapes has less to get wrong. It refuses
// indefinite lengths, tags and floats, which WebAuthn never uses, and caps how
// deep and how long anything can be, since the bytes come from the browser.

const (
	maxDepth = 8
	maxItems = 256
)

var errCBOR = errors.New("webauthn: malformed CBOR")

type cborReader struct {
	b   []byte
	pos int
}

// decodeCBOR reads one item from b and returns it with the bytes left over.
// Maps come back as map[any]any with int64 or string keys, byte strings as
// []byte, text as string, integers as int64.
func decodeCBOR(b []byte) (any, []byte, error) {
	r := &cborReader{b: b}
	v, err := r.item(0)
	if err != nil {
		return nil, nil, err
	}
	return v, r.b[r.pos:], nil
}

func (r *cborReader) byte() (byte, error) {
	if r.pos >= len(r.b) {
		return 0, errCBOR
	}
	c := r.b[r.pos]
	r.pos++
	return c, nil
}

func (r *cborReader) take(n uint64) ([]byte, error) {
	if n > uint64(len(r.b)-r.pos) {
		return nil, errCBOR
	}
	out := r.b[r.pos : r.pos+int(n)]
	r.pos += int(n)
	return out, nil
}

// arg reads the argument that follows an initial byte's additional information.
func (r *cborReader) arg(info byte) (uint64, error) {
	switch {
	case info < 24:
		return uint64(info), nil
	case info == 24:
		c, err := r.byte()
		return uint64(c), err
	case info == 25:
		b, err := r.take(2)
		if err != nil {
			return 0, err
		}
		return uint64(binary.BigEndian.Uint16(b)), nil
	case info == 26:
		b, err := r.take(4)
		if err != nil {
			return 0, err
		}
		return uint64(binary.BigEndian.Uint32(b)), nil
	case info == 27:
		b, err := r.take(8)
		if err != nil {
			return 0, err
		}
		return binary.BigEndian.Uint64(b), nil
	}
	// 28–30 are reserved; 31 is an indefinite length, which WebAuthn never uses.
	return 0, errCBOR
}

func (r *cborReader) item(depth int) (any, error) {
	if depth > maxDepth {
		return nil, errCBOR
	}
	ib, err := r.byte()
	if err != nil {
		return nil, err
	}
	major, info := ib>>5, ib&0x1f
	if major == 7 {
		switch info {
		case 20:
			return false, nil
		case 21:
			return true, nil
		case 22, 23:
			return nil, nil
		}
		return nil, fmt.Errorf("%w: unsupported simple value %d", errCBOR, info)
	}
	n, err := r.arg(info)
	if err != nil {
		return nil, err
	}
	switch major {
	case 0:
		if n > 1<<63-1 {
			return nil, errCBOR
		}
		return int64(n), nil
	case 1:
		if n > 1<<63-1 {
			return nil, errCBOR
		}
		return -1 - int64(n), nil
	case 2:
		b, err := r.take(n)
		if err != nil {
			return nil, err
		}
		return append([]byte(nil), b...), nil
	case 3:
		b, err := r.take(n)
		if err != nil {
			return nil, err
		}
		return string(b), nil
	case 4:
		if n > maxItems {
			return nil, errCBOR
		}
		out := make([]any, 0, n)
		for i := uint64(0); i < n; i++ {
			v, err := r.item(depth + 1)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case 5:
		if n > maxItems {
			return nil, errCBOR
		}
		out := make(map[any]any, n)
		for i := uint64(0); i < n; i++ {
			k, err := r.item(depth + 1)
			if err != nil {
				return nil, err
			}
			switch k.(type) {
			case int64, string:
			default:
				return nil, fmt.Errorf("%w: map key of an unexpected kind", errCBOR)
			}
			v, err := r.item(depth + 1)
			if err != nil {
				return nil, err
			}
			if _, dup := out[k]; dup {
				return nil, fmt.Errorf("%w: a key twice", errCBOR)
			}
			out[k] = v
		}
		return out, nil
	}
	// 6: tags — not used by WebAuthn.
	return nil, fmt.Errorf("%w: unsupported major type %d", errCBOR, major)
}
