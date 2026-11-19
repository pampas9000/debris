package pixcall

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/bits"
)

const (
	keystoreKeySize   = 32
	keystoreNonceSize = 12
	keystoreTagSize   = 16
)

// keystoreMasterKey is Pixcall's fixed 32-byte keystore encryption key.
var keystoreMasterKey = [keystoreKeySize]byte{
	'O', 'p', 'o', 'H', 'r', 'U', 's', 'N',
	'E', 'u', 'u', 'P', 'X', 'i', 'W', 'e',
	'S', '1', 's', 'l', 'v', 'T', 'd', '3',
	'J', '8', '8', 'n', 't', 'P', 'M', 't',
}

type keystoreStatus struct {
	Status string `json:"status"`
}

type keystorePayload struct {
	IsLocal    bool           `json:"is_local"`
	LastActive int64          `json:"last_active"`
	LibraryID  string         `json:"library_id"`
	Status     keystoreStatus `json:"status"`
}

func buildKeystoreBytes(libraryID string, lastActiveUnix int64) ([]byte, error) {
	payload := keystorePayload{
		IsLocal:    true,
		LastActive: lastActiveUnix,
		LibraryID:  libraryID,
		Status: keystoreStatus{
			Status: "ready",
		},
	}
	plaintext, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("pixcall: marshal keystore payload: %w", err)
	}
	return sealKeystore(plaintext)
}

// sealKeystore encrypts plaintext using RFC 8439 ChaCha20-Poly1305 with
// Pixcall's keystore layout: ciphertext || tag(16) || nonce(12).
func sealKeystore(plaintext []byte) ([]byte, error) {
	var nonce [keystoreNonceSize]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("pixcall: generate keystore nonce: %w", err)
	}
	return sealKeystoreWithNonce(plaintext, nonce), nil
}

func sealKeystoreWithNonce(plaintext []byte, nonce [keystoreNonceSize]byte) []byte {
	var otk [32]byte
	polyBlock := chacha20Block(keystoreMasterKey, 0, nonce)
	copy(otk[:], polyBlock[:32])

	ciphertext := make([]byte, len(plaintext))
	chacha20XOR(ciphertext, plaintext, keystoreMasterKey, 1, nonce)

	tag := poly1305AEADTag(otk, nil, ciphertext)

	out := make([]byte, 0, len(ciphertext)+keystoreTagSize+keystoreNonceSize)
	out = append(out, ciphertext...)
	out = append(out, tag[:]...)
	out = append(out, nonce[:]...)
	return out
}

// openKeystore decrypts and authenticates a Pixcall keystore blob.
func openKeystore(blob []byte) ([]byte, error) {
	minLen := keystoreTagSize + keystoreNonceSize
	if len(blob) < minLen {
		return nil, errors.New("pixcall: keystore blob too short")
	}

	nonceOffset := len(blob) - keystoreNonceSize
	tagOffset := nonceOffset - keystoreTagSize
	ciphertext := blob[:tagOffset]
	tag := blob[tagOffset:nonceOffset]

	var nonce [keystoreNonceSize]byte
	copy(nonce[:], blob[nonceOffset:])

	var otk [32]byte
	polyBlock := chacha20Block(keystoreMasterKey, 0, nonce)
	copy(otk[:], polyBlock[:32])

	expectedTag := poly1305AEADTag(otk, nil, ciphertext)
	if subtle.ConstantTimeCompare(tag, expectedTag[:]) != 1 {
		return nil, errors.New("pixcall: keystore authentication failed")
	}

	plaintext := make([]byte, len(ciphertext))
	chacha20XOR(plaintext, ciphertext, keystoreMasterKey, 1, nonce)
	return plaintext, nil
}

func chacha20QuarterRound(s *[16]uint32, a, b, c, d int) {
	s[a] += s[b]
	s[d] = bits.RotateLeft32(s[d]^s[a], 16)
	s[c] += s[d]
	s[b] = bits.RotateLeft32(s[b]^s[c], 12)
	s[a] += s[b]
	s[d] = bits.RotateLeft32(s[d]^s[a], 8)
	s[c] += s[d]
	s[b] = bits.RotateLeft32(s[b]^s[c], 7)
}

func chacha20Block(key [32]byte, counter uint32, nonce [12]byte) [64]byte {
	var state [16]uint32
	state[0] = 0x61707865
	state[1] = 0x3320646e
	state[2] = 0x79622d32
	state[3] = 0x6b206574
	for i := 0; i < 8; i++ {
		state[4+i] = binary.LittleEndian.Uint32(key[i*4:])
	}
	state[12] = counter
	state[13] = binary.LittleEndian.Uint32(nonce[0:])
	state[14] = binary.LittleEndian.Uint32(nonce[4:])
	state[15] = binary.LittleEndian.Uint32(nonce[8:])

	w := state
	for i := 0; i < 10; i++ {
		chacha20QuarterRound(&w, 0, 4, 8, 12)
		chacha20QuarterRound(&w, 1, 5, 9, 13)
		chacha20QuarterRound(&w, 2, 6, 10, 14)
		chacha20QuarterRound(&w, 3, 7, 11, 15)
		chacha20QuarterRound(&w, 0, 5, 10, 15)
		chacha20QuarterRound(&w, 1, 6, 11, 12)
		chacha20QuarterRound(&w, 2, 7, 8, 13)
		chacha20QuarterRound(&w, 3, 4, 9, 14)
	}

	var out [64]byte
	for i := 0; i < 16; i++ {
		binary.LittleEndian.PutUint32(out[i*4:], w[i]+state[i])
	}
	return out
}

func chacha20XOR(dst, src []byte, key [32]byte, counter uint32, nonce [12]byte) {
	for offset := 0; offset < len(src); offset += 64 {
		block := chacha20Block(key, counter, nonce)
		counter++
		n := len(src) - offset
		if n > 64 {
			n = 64
		}
		for i := 0; i < n; i++ {
			dst[offset+i] = src[offset+i] ^ block[i]
		}
	}
}

func poly1305AEADTag(otk [32]byte, aad, ciphertext []byte) [16]byte {
	r := [5]uint64{
		uint64(binary.LittleEndian.Uint32(otk[0:4])) & 0x03ffffff,
		(uint64(binary.LittleEndian.Uint32(otk[3:7])) >> 2) & 0x03ffff03,
		(uint64(binary.LittleEndian.Uint32(otk[6:10])) >> 4) & 0x03ffc0ff,
		(uint64(binary.LittleEndian.Uint32(otk[9:13])) >> 6) & 0x03f03fff,
		(uint64(binary.LittleEndian.Uint32(otk[12:16])) >> 8) & 0x000fffff,
	}
	s := [4]uint64{
		r[1] * 5,
		r[2] * 5,
		r[3] * 5,
		r[4] * 5,
	}
	var h [5]uint64

	processBlock := func(block []byte, hibit uint64) {
		var buf [16]byte
		copy(buf[:], block)
		if len(block) < 16 {
			buf[len(block)] = 1
		}
		t0 := uint64(binary.LittleEndian.Uint32(buf[0:4]))
		t1 := uint64(binary.LittleEndian.Uint32(buf[4:8]))
		t2 := uint64(binary.LittleEndian.Uint32(buf[8:12]))
		t3 := uint64(binary.LittleEndian.Uint32(buf[12:16]))

		h[0] += t0 & 0x03ffffff
		h[1] += ((t0 >> 26) | (t1 << 6)) & 0x03ffffff
		h[2] += ((t1 >> 20) | (t2 << 12)) & 0x03ffffff
		h[3] += ((t2 >> 14) | (t3 << 18)) & 0x03ffffff
		h[4] += (t3 >> 8) | hibit

		d0 := h[0]*r[0] + h[1]*s[3] + h[2]*s[2] + h[3]*s[1] + h[4]*s[0]
		d1 := h[0]*r[1] + h[1]*r[0] + h[2]*s[3] + h[3]*s[2] + h[4]*s[1]
		d2 := h[0]*r[2] + h[1]*r[1] + h[2]*r[0] + h[3]*s[3] + h[4]*s[2]
		d3 := h[0]*r[3] + h[1]*r[2] + h[2]*r[1] + h[3]*r[0] + h[4]*s[3]
		d4 := h[0]*r[4] + h[1]*r[3] + h[2]*r[2] + h[3]*r[1] + h[4]*r[0]

		c := d0 >> 26
		h[0] = d0 & 0x03ffffff
		d1 += c
		c = d1 >> 26
		h[1] = d1 & 0x03ffffff
		d2 += c
		c = d2 >> 26
		h[2] = d2 & 0x03ffffff
		d3 += c
		c = d3 >> 26
		h[3] = d3 & 0x03ffffff
		d4 += c
		c = d4 >> 26
		h[4] = d4 & 0x03ffffff
		h[0] += c * 5
		c = h[0] >> 26
		h[0] &= 0x03ffffff
		h[1] += c
	}

	feedPadded := func(data []byte) {
		for off := 0; off < len(data); off += 16 {
			end := off + 16
			if end <= len(data) {
				processBlock(data[off:end], 1<<24)
			} else {
				var padded [16]byte
				copy(padded[:], data[off:])
				processBlock(padded[:], 1<<24)
			}
		}
	}

	feedPadded(aad)
	feedPadded(ciphertext)

	var lens [16]byte
	binary.LittleEndian.PutUint64(lens[0:8], uint64(len(aad)))
	binary.LittleEndian.PutUint64(lens[8:16], uint64(len(ciphertext)))
	processBlock(lens[:], 1<<24)

	// Finalize mod (2^130 - 5)
	c := h[1] >> 26
	h[1] &= 0x03ffffff
	h[2] += c
	c = h[2] >> 26
	h[2] &= 0x03ffffff
	h[3] += c
	c = h[3] >> 26
	h[3] &= 0x03ffffff
	h[4] += c
	c = h[4] >> 26
	h[4] &= 0x03ffffff
	h[0] += c * 5
	c = h[0] >> 26
	h[0] &= 0x03ffffff
	h[1] += c

	var g [5]uint64
	g[0] = h[0] + 5
	c = g[0] >> 26
	g[0] &= 0x03ffffff
	g[1] = h[1] + c
	c = g[1] >> 26
	g[1] &= 0x03ffffff
	g[2] = h[2] + c
	c = g[2] >> 26
	g[2] &= 0x03ffffff
	g[3] = h[3] + c
	c = g[3] >> 26
	g[3] &= 0x03ffffff
	g[4] = h[4] + c - (1 << 26)

	mask := (g[4] >> 63) - 1
	for i := 0; i < 5; i++ {
		h[i] = (h[i] &^ mask) | (g[i] & mask)
	}

	h0 := (h[0] | (h[1] << 26)) & 0xffffffff
	h1 := ((h[1] >> 6) | (h[2] << 20)) & 0xffffffff
	h2 := ((h[2] >> 12) | (h[3] << 14)) & 0xffffffff
	h3 := ((h[3] >> 18) | (h[4] << 8)) & 0xffffffff

	f0 := h0 + uint64(binary.LittleEndian.Uint32(otk[16:20]))
	f1 := h1 + uint64(binary.LittleEndian.Uint32(otk[20:24])) + (f0 >> 32)
	f2 := h2 + uint64(binary.LittleEndian.Uint32(otk[24:28])) + (f1 >> 32)
	f3 := h3 + uint64(binary.LittleEndian.Uint32(otk[28:32])) + (f2 >> 32)

	var tag [16]byte
	binary.LittleEndian.PutUint32(tag[0:4], uint32(f0))
	binary.LittleEndian.PutUint32(tag[4:8], uint32(f1))
	binary.LittleEndian.PutUint32(tag[8:12], uint32(f2))
	binary.LittleEndian.PutUint32(tag[12:16], uint32(f3))
	return tag
}
