// Copyright (c) 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package edwards25519

import (
	"crypto/subtle"
	"encoding/binary"
	"errors"
)

// A Scalar is an integer modulo
//
//     l = 2^252 + 27742317777372353535851937790883648493
//
// which is the prime order of the edwards25519 group.
//
// This type works similarly to math/big.Int, and all arguments and
// receivers are allowed to alias.
//
// The zero value is a valid zero element.
type Scalar struct {
	// A Scalar is an integer modulo l = 2^252 + 27742317777372353535851937790883648493.
	// Internally, this implementation keeps the scalar in the Montgomery domain.
	s fiat_sc255_montgomery_domain_field_element
}

var (
	scZeroBytes     = [32]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	scOneBytes      = [32]byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	scMinusOneBytes = [32]byte{236, 211, 245, 92, 26, 99, 18, 88, 214, 156, 247, 162, 222, 249, 222, 20, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 16}

	scOne      = Scalar{[4]uint64{0xd6ec31748d98951d, 0xc6ef5bf4737dcf70, 0xfffffffffffffffe, 0xfffffffffffffff}}
	scMinusOne = Scalar{[4]uint64{0x812631a5cf5d3ed0, 0x4def9dea2f79cd65, 1, 0}}
)

// NewScalar returns a new zero Scalar.
func NewScalar() *Scalar {
	return &Scalar{}
}

// MultiplyAdd sets s = x * y + z mod l, and returns s. DEPRECATED. Use the individual Add and Multiply methods instead.
func (s *Scalar) MultiplyAdd(x, y, z *Scalar) *Scalar {
	return s.Multiply(x, y).Add(s, z)
}

// Add sets s = x + y mod l, and returns s.
func (s *Scalar) Add(x, y *Scalar) *Scalar {
	// s = 1 * x + y mod l
	fiat_sc255_add(&s.s, &x.s, &y.s)
	return s
}

// Subtract sets s = x - y mod l, and returns s.
func (s *Scalar) Subtract(x, y *Scalar) *Scalar {
	// s = -1 * y + x mod l
	fiat_sc255_sub(&s.s, &x.s, &y.s)
	return s
}

// Negate sets s = -x mod l, and returns s.
func (s *Scalar) Negate(x *Scalar) *Scalar {
	// s = -1 * x + 0 mod l
	fiat_sc255_opp(&s.s, &x.s)
	return s
}

// Multiply sets s = x * y mod l, and returns s.
func (s *Scalar) Multiply(x, y *Scalar) *Scalar {
	// s = x * y + 0 mod l
	fiat_sc255_mul(&s.s, &x.s, &y.s)
	return s
}

// Set sets s = x, and returns s.
func (s *Scalar) Set(x *Scalar) *Scalar {
	*s = *x
	return s
}

// SetUniformBytes sets s = x mod l, where x is a 64-byte little-endian integer.
// If x is not of the right length, SetUniformBytes returns nil and an error,
// and the receiver is unchanged.
//
// SetUniformBytes can be used to set s to an uniformly distributed value given
// 64 uniformly distributed random bytes.
func (s *Scalar) SetUniformBytes(x []byte) (*Scalar, error) {
	if len(x) != 64 {
		return nil, errors.New("edwards25519: invalid SetUniformBytes input length")
	}
	var wideBytes [64]byte
	copy(wideBytes[:], x[:])

	// TODO: We should deprecate scReduce as well, but we retain it here for consistent behavior
	var reduced [32]byte
	scReduce(&reduced, &wideBytes)

	fiat_sc255_from_bytes((*[4]uint64)(&s.s), &reduced)
	fiat_sc255_to_montgomery(&s.s, (*fiat_sc255_non_montgomery_domain_field_element)(&s.s))

	return s, nil
}

// SetCanonicalBytes sets s = x, where x is a 32-byte little-endian encoding of
// s, and returns s. If x is not a canonical encoding of s, SetCanonicalBytes
// returns nil and an error, and the receiver is unchanged.
func (s *Scalar) SetCanonicalBytes(x []byte) (*Scalar, error) {
	if len(x) != 32 {
		return nil, errors.New("invalid scalar length")
	}

	// Use bytes here because the original logic assumed the old 32-byte LE representation
	ss := [32]byte{}
	copy(ss[:], x)
	if !isReduced(ss[:]) {
		return nil, errors.New("invalid scalar encoding")
	}

	fiat_sc255_from_bytes((*[4]uint64)(&s.s), &ss)
	fiat_sc255_to_montgomery(&s.s, (*fiat_sc255_non_montgomery_domain_field_element)(&s.s))

	return s, nil
}

// isReduced returns whether the given scalar in 32-byte little endian encoded form is reduced modulo l.
func isReduced(s []byte) bool {
	if len(s) != 32 {
		return false
	}

	for i := len(s) - 1; i >= 0; i-- {
		switch {
		case s[i] > scMinusOneBytes[i]:
			return false
		case s[i] < scMinusOneBytes[i]:
			return true
		}
	}
	return true
}

// SetBytesWithClamping applies the buffer pruning described in RFC 8032,
// Section 5.1.5 (also known as clamping) and sets s to the result. The input
// must be 32 bytes, and it is not modified. If x is not of the right length,
// SetBytesWithClamping returns nil and an error, and the receiver is unchanged.
//
// Note that since Scalar values are always reduced modulo the prime order of
// the curve, the resulting value will not preserve any of the cofactor-clearing
// properties that clamping is meant to provide. It will however work as
// expected as long as it is applied to points on the prime order subgroup, like
// in Ed25519. In fact, it is lost to history why RFC 8032 adopted the
// irrelevant RFC 7748 clamping, but it is now required for compatibility.
func (s *Scalar) SetBytesWithClamping(x []byte) (*Scalar, error) {
	// The description above omits the purpose of the high bits of the clamping
	// for brevity, but those are also lost to reductions, and are also
	// irrelevant to edwards25519 as they protect against a specific
	// implementation bug that was once observed in a generic Montgomery ladder.
	if len(x) != 32 {
		return nil, errors.New("edwards25519: invalid SetBytesWithClamping input length")
	}
	var wideBytes [64]byte
	copy(wideBytes[:], x[:])
	wideBytes[0] &= 248
	wideBytes[31] &= 63
	wideBytes[31] |= 64
	var reduced [32]byte
	scReduce(&reduced, &wideBytes)
	fiat_sc255_from_bytes((*[4]uint64)(&s.s), &reduced)
	fiat_sc255_to_montgomery(&s.s, (*fiat_sc255_non_montgomery_domain_field_element)(&s.s))
	return s, nil
}

// Bytes returns the canonical 32-byte little-endian encoding of s.
func (s *Scalar) Bytes() []byte {
	// This pattern, called "outlining", allows this function to inline so the
	// allocations can occur on the caller stack rather than escaping to the heap.
	// See https://blog.filippo.io/efficient-go-apis-with-the-inliner for more details.
	var encoded [32]byte
	return s.bytes(&encoded)
}

func (s *Scalar) bytes(out *[32]byte) []byte {
	var limbs fiat_sc255_non_montgomery_domain_field_element
	fiat_sc255_from_montgomery(&limbs, &s.s)
	fiat_sc255_to_bytes(out, &limbs)
	return out[:]
}

// Equal returns 1 if s and t are equal, and 0 otherwise.
func (s *Scalar) Equal(t *Scalar) int {
	st := t.Bytes()
	ss := s.Bytes()
	return subtle.ConstantTimeCompare(ss[:], st[:])
}

// scMulAdd and scReduce are ported from the public domain, “ref10”
// implementation of ed25519 from SUPERCOP.

func load3(in []byte) int64 {
	r := int64(in[0])
	r |= int64(in[1]) << 8
	r |= int64(in[2]) << 16
	return r
}

func load4(in []byte) int64 {
	r := int64(in[0])
	r |= int64(in[1]) << 8
	r |= int64(in[2]) << 16
	r |= int64(in[3]) << 24
	return r
}

// Input:
//   s[0]+256*s[1]+...+256^63*s[63] = s
//
// Output:
//   s[0]+256*s[1]+...+256^31*s[31] = s mod l
//   where l = 2^252 + 27742317777372353535851937790883648493.
func scReduce(out *[32]byte, s *[64]byte) {
	s0 := 2097151 & load3(s[:])
	s1 := 2097151 & (load4(s[2:]) >> 5)
	s2 := 2097151 & (load3(s[5:]) >> 2)
	s3 := 2097151 & (load4(s[7:]) >> 7)
	s4 := 2097151 & (load4(s[10:]) >> 4)
	s5 := 2097151 & (load3(s[13:]) >> 1)
	s6 := 2097151 & (load4(s[15:]) >> 6)
	s7 := 2097151 & (load3(s[18:]) >> 3)
	s8 := 2097151 & load3(s[21:])
	s9 := 2097151 & (load4(s[23:]) >> 5)
	s10 := 2097151 & (load3(s[26:]) >> 2)
	s11 := 2097151 & (load4(s[28:]) >> 7)
	s12 := 2097151 & (load4(s[31:]) >> 4)
	s13 := 2097151 & (load3(s[34:]) >> 1)
	s14 := 2097151 & (load4(s[36:]) >> 6)
	s15 := 2097151 & (load3(s[39:]) >> 3)
	s16 := 2097151 & load3(s[42:])
	s17 := 2097151 & (load4(s[44:]) >> 5)
	s18 := 2097151 & (load3(s[47:]) >> 2)
	s19 := 2097151 & (load4(s[49:]) >> 7)
	s20 := 2097151 & (load4(s[52:]) >> 4)
	s21 := 2097151 & (load3(s[55:]) >> 1)
	s22 := 2097151 & (load4(s[57:]) >> 6)
	s23 := (load4(s[60:]) >> 3)

	s11 += s23 * 666643
	s12 += s23 * 470296
	s13 += s23 * 654183
	s14 -= s23 * 997805
	s15 += s23 * 136657
	s16 -= s23 * 683901
	s23 = 0

	s10 += s22 * 666643
	s11 += s22 * 470296
	s12 += s22 * 654183
	s13 -= s22 * 997805
	s14 += s22 * 136657
	s15 -= s22 * 683901
	s22 = 0

	s9 += s21 * 666643
	s10 += s21 * 470296
	s11 += s21 * 654183
	s12 -= s21 * 997805
	s13 += s21 * 136657
	s14 -= s21 * 683901
	s21 = 0

	s8 += s20 * 666643
	s9 += s20 * 470296
	s10 += s20 * 654183
	s11 -= s20 * 997805
	s12 += s20 * 136657
	s13 -= s20 * 683901
	s20 = 0

	s7 += s19 * 666643
	s8 += s19 * 470296
	s9 += s19 * 654183
	s10 -= s19 * 997805
	s11 += s19 * 136657
	s12 -= s19 * 683901
	s19 = 0

	s6 += s18 * 666643
	s7 += s18 * 470296
	s8 += s18 * 654183
	s9 -= s18 * 997805
	s10 += s18 * 136657
	s11 -= s18 * 683901
	s18 = 0

	var carry [17]int64

	carry[6] = (s6 + (1 << 20)) >> 21
	s7 += carry[6]
	s6 -= carry[6] << 21
	carry[8] = (s8 + (1 << 20)) >> 21
	s9 += carry[8]
	s8 -= carry[8] << 21
	carry[10] = (s10 + (1 << 20)) >> 21
	s11 += carry[10]
	s10 -= carry[10] << 21
	carry[12] = (s12 + (1 << 20)) >> 21
	s13 += carry[12]
	s12 -= carry[12] << 21
	carry[14] = (s14 + (1 << 20)) >> 21
	s15 += carry[14]
	s14 -= carry[14] << 21
	carry[16] = (s16 + (1 << 20)) >> 21
	s17 += carry[16]
	s16 -= carry[16] << 21

	carry[7] = (s7 + (1 << 20)) >> 21
	s8 += carry[7]
	s7 -= carry[7] << 21
	carry[9] = (s9 + (1 << 20)) >> 21
	s10 += carry[9]
	s9 -= carry[9] << 21
	carry[11] = (s11 + (1 << 20)) >> 21
	s12 += carry[11]
	s11 -= carry[11] << 21
	carry[13] = (s13 + (1 << 20)) >> 21
	s14 += carry[13]
	s13 -= carry[13] << 21
	carry[15] = (s15 + (1 << 20)) >> 21
	s16 += carry[15]
	s15 -= carry[15] << 21

	s5 += s17 * 666643
	s6 += s17 * 470296
	s7 += s17 * 654183
	s8 -= s17 * 997805
	s9 += s17 * 136657
	s10 -= s17 * 683901
	s17 = 0

	s4 += s16 * 666643
	s5 += s16 * 470296
	s6 += s16 * 654183
	s7 -= s16 * 997805
	s8 += s16 * 136657
	s9 -= s16 * 683901
	s16 = 0

	s3 += s15 * 666643
	s4 += s15 * 470296
	s5 += s15 * 654183
	s6 -= s15 * 997805
	s7 += s15 * 136657
	s8 -= s15 * 683901
	s15 = 0

	s2 += s14 * 666643
	s3 += s14 * 470296
	s4 += s14 * 654183
	s5 -= s14 * 997805
	s6 += s14 * 136657
	s7 -= s14 * 683901
	s14 = 0

	s1 += s13 * 666643
	s2 += s13 * 470296
	s3 += s13 * 654183
	s4 -= s13 * 997805
	s5 += s13 * 136657
	s6 -= s13 * 683901
	s13 = 0

	s0 += s12 * 666643
	s1 += s12 * 470296
	s2 += s12 * 654183
	s3 -= s12 * 997805
	s4 += s12 * 136657
	s5 -= s12 * 683901
	s12 = 0

	carry[0] = (s0 + (1 << 20)) >> 21
	s1 += carry[0]
	s0 -= carry[0] << 21
	carry[2] = (s2 + (1 << 20)) >> 21
	s3 += carry[2]
	s2 -= carry[2] << 21
	carry[4] = (s4 + (1 << 20)) >> 21
	s5 += carry[4]
	s4 -= carry[4] << 21
	carry[6] = (s6 + (1 << 20)) >> 21
	s7 += carry[6]
	s6 -= carry[6] << 21
	carry[8] = (s8 + (1 << 20)) >> 21
	s9 += carry[8]
	s8 -= carry[8] << 21
	carry[10] = (s10 + (1 << 20)) >> 21
	s11 += carry[10]
	s10 -= carry[10] << 21

	carry[1] = (s1 + (1 << 20)) >> 21
	s2 += carry[1]
	s1 -= carry[1] << 21
	carry[3] = (s3 + (1 << 20)) >> 21
	s4 += carry[3]
	s3 -= carry[3] << 21
	carry[5] = (s5 + (1 << 20)) >> 21
	s6 += carry[5]
	s5 -= carry[5] << 21
	carry[7] = (s7 + (1 << 20)) >> 21
	s8 += carry[7]
	s7 -= carry[7] << 21
	carry[9] = (s9 + (1 << 20)) >> 21
	s10 += carry[9]
	s9 -= carry[9] << 21
	carry[11] = (s11 + (1 << 20)) >> 21
	s12 += carry[11]
	s11 -= carry[11] << 21

	s0 += s12 * 666643
	s1 += s12 * 470296
	s2 += s12 * 654183
	s3 -= s12 * 997805
	s4 += s12 * 136657
	s5 -= s12 * 683901
	s12 = 0

	carry[0] = s0 >> 21
	s1 += carry[0]
	s0 -= carry[0] << 21
	carry[1] = s1 >> 21
	s2 += carry[1]
	s1 -= carry[1] << 21
	carry[2] = s2 >> 21
	s3 += carry[2]
	s2 -= carry[2] << 21
	carry[3] = s3 >> 21
	s4 += carry[3]
	s3 -= carry[3] << 21
	carry[4] = s4 >> 21
	s5 += carry[4]
	s4 -= carry[4] << 21
	carry[5] = s5 >> 21
	s6 += carry[5]
	s5 -= carry[5] << 21
	carry[6] = s6 >> 21
	s7 += carry[6]
	s6 -= carry[6] << 21
	carry[7] = s7 >> 21
	s8 += carry[7]
	s7 -= carry[7] << 21
	carry[8] = s8 >> 21
	s9 += carry[8]
	s8 -= carry[8] << 21
	carry[9] = s9 >> 21
	s10 += carry[9]
	s9 -= carry[9] << 21
	carry[10] = s10 >> 21
	s11 += carry[10]
	s10 -= carry[10] << 21
	carry[11] = s11 >> 21
	s12 += carry[11]
	s11 -= carry[11] << 21

	s0 += s12 * 666643
	s1 += s12 * 470296
	s2 += s12 * 654183
	s3 -= s12 * 997805
	s4 += s12 * 136657
	s5 -= s12 * 683901
	s12 = 0

	carry[0] = s0 >> 21
	s1 += carry[0]
	s0 -= carry[0] << 21
	carry[1] = s1 >> 21
	s2 += carry[1]
	s1 -= carry[1] << 21
	carry[2] = s2 >> 21
	s3 += carry[2]
	s2 -= carry[2] << 21
	carry[3] = s3 >> 21
	s4 += carry[3]
	s3 -= carry[3] << 21
	carry[4] = s4 >> 21
	s5 += carry[4]
	s4 -= carry[4] << 21
	carry[5] = s5 >> 21
	s6 += carry[5]
	s5 -= carry[5] << 21
	carry[6] = s6 >> 21
	s7 += carry[6]
	s6 -= carry[6] << 21
	carry[7] = s7 >> 21
	s8 += carry[7]
	s7 -= carry[7] << 21
	carry[8] = s8 >> 21
	s9 += carry[8]
	s8 -= carry[8] << 21
	carry[9] = s9 >> 21
	s10 += carry[9]
	s9 -= carry[9] << 21
	carry[10] = s10 >> 21
	s11 += carry[10]
	s10 -= carry[10] << 21

	out[0] = byte(s0 >> 0)
	out[1] = byte(s0 >> 8)
	out[2] = byte((s0 >> 16) | (s1 << 5))
	out[3] = byte(s1 >> 3)
	out[4] = byte(s1 >> 11)
	out[5] = byte((s1 >> 19) | (s2 << 2))
	out[6] = byte(s2 >> 6)
	out[7] = byte((s2 >> 14) | (s3 << 7))
	out[8] = byte(s3 >> 1)
	out[9] = byte(s3 >> 9)
	out[10] = byte((s3 >> 17) | (s4 << 4))
	out[11] = byte(s4 >> 4)
	out[12] = byte(s4 >> 12)
	out[13] = byte((s4 >> 20) | (s5 << 1))
	out[14] = byte(s5 >> 7)
	out[15] = byte((s5 >> 15) | (s6 << 6))
	out[16] = byte(s6 >> 2)
	out[17] = byte(s6 >> 10)
	out[18] = byte((s6 >> 18) | (s7 << 3))
	out[19] = byte(s7 >> 5)
	out[20] = byte(s7 >> 13)
	out[21] = byte(s8 >> 0)
	out[22] = byte(s8 >> 8)
	out[23] = byte((s8 >> 16) | (s9 << 5))
	out[24] = byte(s9 >> 3)
	out[25] = byte(s9 >> 11)
	out[26] = byte((s9 >> 19) | (s10 << 2))
	out[27] = byte(s10 >> 6)
	out[28] = byte((s10 >> 14) | (s11 << 7))
	out[29] = byte(s11 >> 1)
	out[30] = byte(s11 >> 9)
	out[31] = byte(s11 >> 17)
}

// nonAdjacentForm computes a width-w non-adjacent form for this scalar.
//
// w must be between 2 and 8, or nonAdjacentForm will panic.
func (s *Scalar) nonAdjacentForm(w uint) [256]int8 {
	// This implementation is adapted from the one
	// in curve25519-dalek and is documented there:
	// https://github.com/dalek-cryptography/curve25519-dalek/blob/f630041af28e9a405255f98a8a93adca18e4315b/src/scalar.rs#L800-L871
	b := s.Bytes()
	if b[31] > 127 {
		panic("scalar has high bit set illegally")
	}
	if w < 2 {
		panic("w must be at least 2 by the definition of NAF")
	} else if w > 8 {
		panic("NAF digits must fit in int8")
	}

	var naf [256]int8
	var digits [5]uint64

	for i := 0; i < 4; i++ {
		digits[i] = binary.LittleEndian.Uint64(b[i*8:])
	}

	width := uint64(1 << w)
	windowMask := uint64(width - 1)

	pos := uint(0)
	carry := uint64(0)
	for pos < 256 {
		indexU64 := pos / 64
		indexBit := pos % 64
		var bitBuf uint64
		if indexBit < 64-w {
			// This window's bits are contained in a single u64
			bitBuf = digits[indexU64] >> indexBit
		} else {
			// Combine the current 64 bits with bits from the next 64
			bitBuf = (digits[indexU64] >> indexBit) | (digits[1+indexU64] << (64 - indexBit))
		}

		// Add carry into the current window
		window := carry + (bitBuf & windowMask)

		if window&1 == 0 {
			// If the window value is even, preserve the carry and continue.
			// Why is the carry preserved?
			// If carry == 0 and window & 1 == 0,
			//    then the next carry should be 0
			// If carry == 1 and window & 1 == 0,
			//    then bit_buf & 1 == 1 so the next carry should be 1
			pos += 1
			continue
		}

		if window < width/2 {
			carry = 0
			naf[pos] = int8(window)
		} else {
			carry = 1
			naf[pos] = int8(window) - int8(width)
		}

		pos += w
	}
	return naf
}

func (s *Scalar) signedRadix16() [64]int8 {
	b := s.Bytes()
	if b[31] > 127 {
		panic("scalar has high bit set illegally")
	}

	var digits [64]int8

	// Compute unsigned radix-16 digits:
	for i := 0; i < 32; i++ {
		digits[2*i] = int8(b[i] & 15)
		digits[2*i+1] = int8((b[i] >> 4) & 15)
	}

	// Recenter coefficients:
	for i := 0; i < 63; i++ {
		carry := (digits[i] + 8) >> 4
		digits[i] -= carry << 4
		digits[i+1] += carry
	}

	return digits
}
