// Copyright (c) 2017 George Tankersley. All rights reserved.
// Copyright (c) 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package edwards25519 implements group logic for the twisted Edwards curve
//
//     -x^2 + y^2 = 1 + -(121665/121666)*x^2*y^2
//
// This is better known as the Edwards curve equivalent to curve25519, and is
// the curve used by the Ed25519 signature scheme.
package edwards25519

import (
	"math/big"

	"github.com/gtank/ristretto255/internal/radix51"
)

// D is a constant in the curve equation.
var D = &radix51.FieldElement{929955233495203, 466365720129213,
	1662059464998953, 2033849074728123, 1442794654840575}

// Point types.

type ProjP1xP1 struct {
	X, Y, Z, T radix51.FieldElement
}

type ProjP2 struct {
	X, Y, Z radix51.FieldElement
}

type ProjP3 struct {
	X, Y, Z, T radix51.FieldElement
}

type ProjCached struct {
	YplusX, YminusX, Z, T2d radix51.FieldElement
}

type AffineCached struct {
	YplusX, YminusX, T2d radix51.FieldElement
}

// Constructors.

func (v *ProjP1xP1) Zero() *ProjP1xP1 {
	v.X.Zero()
	v.Y.One()
	v.Z.One()
	v.T.One()
	return v
}

func (v *ProjP2) Zero() *ProjP2 {
	v.X.Zero()
	v.Y.One()
	v.Z.One()
	return v
}

func (v *ProjP3) Zero() *ProjP3 {
	v.X.Zero()
	v.Y.One()
	v.Z.One()
	v.T.Zero()
	return v
}

func (v *ProjCached) Zero() *ProjCached {
	v.YplusX.One()
	v.YminusX.One()
	v.Z.One()
	v.T2d.Zero()
	return v
}

func (v *AffineCached) Zero() *AffineCached {
	v.YplusX.One()
	v.YminusX.One()
	v.T2d.Zero()
	return v
}

// Conversions.

func (v *ProjP2) FromP1xP1(p *ProjP1xP1) *ProjP2 {
	v.X.Mul(&p.X, &p.T)
	v.Y.Mul(&p.Y, &p.Z)
	v.Z.Mul(&p.Z, &p.T)
	return v
}

func (v *ProjP2) FromP3(p *ProjP3) *ProjP2 {
	v.X.Set(&p.X)
	v.Y.Set(&p.Y)
	v.Z.Set(&p.Z)
	return v
}

func (v *ProjP3) FromP1xP1(p *ProjP1xP1) *ProjP3 {
	v.X.Mul(&p.X, &p.T)
	v.Y.Mul(&p.Y, &p.Z)
	v.Z.Mul(&p.Z, &p.T)
	v.T.Mul(&p.X, &p.Y)
	return v
}

func (v *ProjP3) FromP2(p *ProjP2) *ProjP3 {
	v.X.Mul(&p.X, &p.Z)
	v.Y.Mul(&p.Y, &p.Z)
	v.Z.Square(&p.Z)
	v.T.Mul(&p.X, &p.Y)
	return v
}

func (v *ProjCached) FromP3(p *ProjP3) *ProjCached {
	v.YplusX.Add(&p.Y, &p.X)
	v.YminusX.Sub(&p.Y, &p.X)
	v.Z.Set(&p.Z)
	v.T2d.Mul(&p.T, twoD)
	return v
}

func (v *AffineCached) FromP3(p *ProjP3) *AffineCached {
	v.YplusX.Add(&p.Y, &p.X)
	v.YminusX.Sub(&p.Y, &p.X)
	v.T2d.Mul(&p.T, twoD)

	var invZ radix51.FieldElement
	invZ.Invert(&p.Z)
	v.YplusX.Mul(&v.YplusX, &invZ)
	v.YminusX.Mul(&v.YminusX, &invZ)
	v.T2d.Mul(&v.T2d, &invZ)
	return v
}

// (Re)addition and subtraction.

func (v *ProjP3) Add(p, q *ProjP3) *ProjP3 {
	result := ProjP1xP1{}
	qCached := ProjCached{}
	qCached.FromP3(q)
	result.Add(p, &qCached)
	v.FromP1xP1(&result)
	return v
}

func (v *ProjP3) Sub(p, q *ProjP3) *ProjP3 {
	result := ProjP1xP1{}
	qCached := ProjCached{}
	qCached.FromP3(q)
	result.Sub(p, &qCached)
	v.FromP1xP1(&result)
	return v
}

func (v *ProjP1xP1) Add(p *ProjP3, q *ProjCached) *ProjP1xP1 {
	var YplusX, YminusX, PP, MM, TT2d, ZZ2 radix51.FieldElement

	YplusX.Add(&p.Y, &p.X)
	YminusX.Sub(&p.Y, &p.X)

	PP.Mul(&YplusX, &q.YplusX)
	MM.Mul(&YminusX, &q.YminusX)
	TT2d.Mul(&p.T, &q.T2d)
	ZZ2.Mul(&p.Z, &q.Z)

	ZZ2.Add(&ZZ2, &ZZ2)

	v.X.Sub(&PP, &MM)
	v.Y.Add(&PP, &MM)
	v.Z.Add(&ZZ2, &TT2d)
	v.T.Sub(&ZZ2, &TT2d)
	return v
}

func (v *ProjP1xP1) Sub(p *ProjP3, q *ProjCached) *ProjP1xP1 {
	var YplusX, YminusX, PP, MM, TT2d, ZZ2 radix51.FieldElement

	YplusX.Add(&p.Y, &p.X)
	YminusX.Sub(&p.Y, &p.X)

	PP.Mul(&YplusX, &q.YminusX) // flipped sign
	MM.Mul(&YminusX, &q.YplusX) // flipped sign
	TT2d.Mul(&p.T, &q.T2d)
	ZZ2.Mul(&p.Z, &q.Z)

	ZZ2.Add(&ZZ2, &ZZ2)

	v.X.Sub(&PP, &MM)
	v.Y.Add(&PP, &MM)
	v.Z.Sub(&ZZ2, &TT2d) // flipped sign
	v.T.Add(&ZZ2, &TT2d) // flipped sign
	return v
}

func (v *ProjP1xP1) AddAffine(p *ProjP3, q *AffineCached) *ProjP1xP1 {
	var YplusX, YminusX, PP, MM, TT2d, Z2 radix51.FieldElement

	YplusX.Add(&p.Y, &p.X)
	YminusX.Sub(&p.Y, &p.X)

	PP.Mul(&YplusX, &q.YplusX)
	MM.Mul(&YminusX, &q.YminusX)
	TT2d.Mul(&p.T, &q.T2d)

	Z2.Add(&p.Z, &p.Z)

	v.X.Sub(&PP, &MM)
	v.Y.Add(&PP, &MM)
	v.Z.Add(&Z2, &TT2d)
	v.T.Sub(&Z2, &TT2d)
	return v
}

func (v *ProjP1xP1) SubAffine(p *ProjP3, q *AffineCached) *ProjP1xP1 {
	var YplusX, YminusX, PP, MM, TT2d, Z2 radix51.FieldElement

	YplusX.Add(&p.Y, &p.X)
	YminusX.Sub(&p.Y, &p.X)

	PP.Mul(&YplusX, &q.YminusX) // flipped sign
	MM.Mul(&YminusX, &q.YplusX) // flipped sign
	TT2d.Mul(&p.T, &q.T2d)

	Z2.Add(&p.Z, &p.Z)

	v.X.Sub(&PP, &MM)
	v.Y.Add(&PP, &MM)
	v.Z.Sub(&Z2, &TT2d) // flipped sign
	v.T.Add(&Z2, &TT2d) // flipped sign
	return v
}

// Doubling.

func (v *ProjP1xP1) Double(p *ProjP2) *ProjP1xP1 {
	var XX, YY, ZZ2, XplusYsq radix51.FieldElement

	XX.Square(&p.X)
	YY.Square(&p.Y)
	ZZ2.Square(&p.Z)
	ZZ2.Add(&ZZ2, &ZZ2)
	XplusYsq.Add(&p.X, &p.Y)
	XplusYsq.Square(&XplusYsq)

	v.Y.Add(&YY, &XX)
	v.Z.Sub(&YY, &XX)

	v.X.Sub(&XplusYsq, &v.Y)
	v.T.Sub(&ZZ2, &v.Z)
	return v
}

// Negation.

func (v *ProjP3) Neg(p *ProjP3) *ProjP3 {
	v.X.Neg(&p.X)
	v.Y.Set(&p.Y)
	v.Z.Set(&p.Z)
	v.T.Neg(&p.T)
	return v
}

// by @ebfull
// https://github.com/dalek-cryptography/curve25519-dalek/pull/226/files
func (v *ProjP3) Equal(u *ProjP3) int {
	var t1, t2, t3, t4 radix51.FieldElement
	t1.Mul(&v.X, &u.Z)
	t2.Mul(&u.X, &v.Z)
	t3.Mul(&v.Y, &u.Z)
	t4.Mul(&u.Y, &v.Z)

	return t1.Equal(&t2) & t3.Equal(&t4)
}

// From EFD https://hyperelliptic.org/EFD/g1p/auto-twisted-extended-1.html
// An elliptic curve in twisted Edwards form has parameters a, d and coordinates
// x, y satisfying the following equations:
//
//     a * x^2 + y^2 = 1 + d * x^2 * y^2
//
// Extended coordinates assume a = -1 and represent x, y as (X, Y, Z, T)
// satisfying the following equations:
//
//     x = X / Z
//     y = Y / Z
//     x * y = T / Z
//
// This representation was introduced in the HisilWongCarterDawson paper "Twisted
// Edwards curves revisited" (Asiacrypt 2008).
type ExtendedGroupElement struct {
	X, Y, Z, T radix51.FieldElement
}

func (v *ExtendedGroupElement) Set(u *ExtendedGroupElement) *ExtendedGroupElement {
	*v = *u
	return v
}

// Converts (x,y) to (X:Y:T:Z) extended coordinates, or "P3" in ref10. As
// described in "Twisted Edwards Curves Revisited", Hisil-Wong-Carter-Dawson
// 2008, Section 3.1 (https://eprint.iacr.org/2008/522.pdf)
// See also https://hyperelliptic.org/EFD/g1p/auto-twisted-extended-1.html#addition-add-2008-hwcd-3
func (v *ExtendedGroupElement) FromAffine(x, y *big.Int) *ExtendedGroupElement {
	v.X.FromBig(x)
	v.Y.FromBig(y)
	v.T.Mul(&v.X, &v.Y)
	v.Z.One()
	return v
}

// Extended coordinates are XYZT with x = X/Z, y = Y/Z, or the "P3"
// representation in ref10. Extended->affine is the same operation as moving
// from projective to affine. Per HWCD, it is safe to move from extended to
// projective by simply ignoring T.
func (v *ExtendedGroupElement) ToAffine() (*big.Int, *big.Int) {
	var x, y, zinv radix51.FieldElement

	zinv.Invert(&v.Z)
	x.Mul(&v.X, &zinv)
	y.Mul(&v.Y, &zinv)

	return x.ToBig(), y.ToBig()
}

// Per HWCD, it is safe to move from extended to projective by simply ignoring T.
func (v *ExtendedGroupElement) ToProjective(p *ProjectiveGroupElement) {
	p.X.Set(&v.X)
	p.Y.Set(&v.Y)
	p.Z.Set(&v.Z)
}

func (v *ExtendedGroupElement) Zero() *ExtendedGroupElement {
	v.X.Zero()
	v.Y.One()
	v.Z.One()
	v.T.Zero()
	return v
}

var twoD = new(radix51.FieldElement).Add(D, D)

// This is the same addition formula everyone uses, "add-2008-hwcd-3".
// https://hyperelliptic.org/EFD/g1p/auto-twisted-extended-1.html#addition-add-2008-hwcd-3
// TODO We know Z1=1 and Z2=1 here, so mmadd-2008-hwcd-3 (6M + 1S + 1*k + 9add) could apply
func (v *ExtendedGroupElement) Add(p1, p2 *ExtendedGroupElement) *ExtendedGroupElement {
	var tmp1, tmp2, A, B, C, D, E, F, G, H radix51.FieldElement
	tmp1.Sub(&p1.Y, &p1.X) // tmp1 <-- Y1-X1
	tmp2.Sub(&p2.Y, &p2.X) // tmp2 <-- Y2-X2
	A.Mul(&tmp1, &tmp2)    // A <-- tmp1*tmp2 = (Y1-X1)*(Y2-X2)
	tmp1.Add(&p1.Y, &p1.X) // tmp1 <-- Y1+X1
	tmp2.Add(&p2.Y, &p2.X) // tmp2 <-- Y2+X2
	B.Mul(&tmp1, &tmp2)    // B <-- tmp1*tmp2 = (Y1+X1)*(Y2+X2)
	tmp1.Mul(&p1.T, &p2.T) // tmp1 <-- T1*T2
	C.Mul(&tmp1, twoD)     // C <-- tmp1*2d = T1*2*d*T2
	tmp1.Mul(&p1.Z, &p2.Z) // tmp1 <-- Z1*Z2
	D.Add(&tmp1, &tmp1)    // D <-- tmp1 + tmp1 = 2*Z1*Z2
	E.Sub(&B, &A)          // E <-- B-A
	F.Sub(&D, &C)          // F <-- D-C
	G.Add(&D, &C)          // G <-- D+C
	H.Add(&B, &A)          // H <-- B+A
	v.X.Mul(&E, &F)        // X3 <-- E*F
	v.Y.Mul(&G, &H)        // Y3 <-- G*H
	v.T.Mul(&E, &H)        // T3 <-- E*H
	v.Z.Mul(&F, &G)        // Z3 <-- F*G
	return v
}

func (v *ExtendedGroupElement) Sub(p1, p2 *ExtendedGroupElement) *ExtendedGroupElement {
	// This is the same function as above, but with X2, T2 negated to X2'=-X2, T2'=-T2
	var tmp1, tmp2, A, B, C, D, E, F, G, H radix51.FieldElement
	tmp1.Sub(&p1.Y, &p1.X) // tmp1 <-- Y1-X1
	tmp2.Add(&p2.Y, &p2.X) // tmp2 <-- Y2+X2 = Y2-X2'
	A.Mul(&tmp1, &tmp2)    // A <-- tmp1*tmp2 = (Y1-X1)*(Y2-X2')
	tmp1.Add(&p1.Y, &p1.X) // tmp1 <-- Y1+X1
	tmp2.Sub(&p2.Y, &p2.X) // tmp2 <-- Y2-X2 = Y2+X2'
	B.Mul(&tmp1, &tmp2)    // B <-- tmp1*tmp2 = (Y1+X1)*(Y2+X2)
	tmp1.Mul(&p1.T, &p2.T) // tmp1 <-- -T1*T2' = T1*T2
	C.Mul(&tmp1, twoD)     // C' <-- tmp1*2d = -T1*2*d*T2' = T1*2*d*T2
	tmp1.Mul(&p1.Z, &p2.Z) // tmp1 <-- Z1*Z2
	D.Add(&tmp1, &tmp1)    // D <-- tmp1 + tmp1 = 2*Z1*Z2
	E.Sub(&B, &A)          // E <-- B-A
	F.Add(&D, &C)          // F <-- D+C' = D-C
	G.Sub(&D, &C)          // G <-- D-C' = D+C
	H.Add(&B, &A)          // H <-- B+A
	v.X.Mul(&E, &F)        // X3 <-- E*F
	v.Y.Mul(&G, &H)        // Y3 <-- G*H
	v.T.Mul(&E, &H)        // T3 <-- E*H
	v.Z.Mul(&F, &G)        // Z3 <-- F*G
	return v
}

func (v *ExtendedGroupElement) Neg(p *ExtendedGroupElement) *ExtendedGroupElement {
	v.X.Neg(&p.X)
	v.Y.Set(&p.Y)
	v.Z.Set(&p.Z)
	v.T.Neg(&p.T)
	return v
}

// by @ebfull
// https://github.com/dalek-cryptography/curve25519-dalek/pull/226/files
func (v *ExtendedGroupElement) Equal(u *ExtendedGroupElement) int {
	var t1, t2, t3, t4 radix51.FieldElement
	t1.Mul(&v.X, &u.Z)
	t2.Mul(&u.X, &v.Z)
	t3.Mul(&v.Y, &u.Z)
	t4.Mul(&u.Y, &v.Z)

	return t1.Equal(&t2) & t3.Equal(&t4)
}

// This implements the explicit formulas from HWCD Section 3.3, "Dedicated
// Doubling in [extended coordinates]".
//
// Explicit formula is as follows. Cost is 4M + 4S + 1D. For Ed25519, a = -1:
//
//       A ← X1^2
//       B ← Y1^2
//       C ← 2*Z1^2
//       D ← a*A
//       E ← (X1+Y1)^2 − A − B
//       G ← D+B
//       F ← G−C
//       H ← D−B
//       X3 ← E*F
//       Y3 ← G*H
//       T3 ← E*H
//       Z3 ← F*G
//
// In ref10/donna/dalek etc, this is instead handled by a faster
// mixed-coordinate doubling that results in a "Completed" group element
// instead of another point in extended coordinates. I have implemented it
// this way to see if more straightforward code is worth the (hopefully small)
// performance tradeoff.
func (v *ExtendedGroupElement) Double(u *ExtendedGroupElement) *ExtendedGroupElement {
	// TODO: Convert to projective coordinates? Section 4.3 mixed doubling?

	var A, B, C, D, E, F, G, H radix51.FieldElement

	// A ← X1^2, B ← Y1^2
	A.Square(&u.X)
	B.Square(&u.Y)

	// C ← 2*Z1^2
	C.Square(&u.Z)
	C.Add(&C, &C) // TODO should probably implement FeSquare2

	// D ← -1*A
	D.Neg(&A) // implemented as subtraction

	// E ← (X1+Y1)^2 − A − B
	var t0 radix51.FieldElement
	t0.Add(&u.X, &u.Y)
	t0.Square(&t0)
	E.Sub(&t0, &A)
	E.Sub(&E, &B)

	G.Add(&D, &B)   // G ← D+B
	F.Sub(&G, &C)   // F ← G−C
	H.Sub(&D, &B)   // H ← D−B
	v.X.Mul(&E, &F) // X3 ← E*F
	v.Y.Mul(&G, &H) // Y3 ← G*H
	v.T.Mul(&E, &H) // T3 ← E*H
	v.Z.Mul(&F, &G) // Z3 ← F*G

	return v
}

// ScalarMult sets v = k*u where k is a reduced scalar field element in
// little-endian form. Note: this function is not constant-time.
func (v *ExtendedGroupElement) ScalarMult(u *ExtendedGroupElement, k *[32]byte) *ExtendedGroupElement {
	// Montgomery ladder init:
	// R_0 = O, R_1 = P
	r1 := new(ExtendedGroupElement).Set(u)
	r0 := v.Zero()

	// Montgomery ladder step:
	// R_{1-b} = R_{1-b} + R_{b}
	// R_{b} = 2*R_{b}
	for i := 255; i >= 0; i-- {
		var b = int32((k[i/8] >> uint(i&7)) & 1)
		if b == 0 {
			r1.Add(r0, r1)
			r0.Double(r0)
		} else {
			r0.Add(r0, r1)
			r1.Double(r1)
		}
	}

	return r0
}

// Projective coordinates are XYZ with x = X/Z, y = Y/Z, or the "P2"
// representation in ref10. This representation has a cheaper doubling formula
// than extended coordinates.
type ProjectiveGroupElement struct {
	X, Y, Z radix51.FieldElement
}

func (v *ProjectiveGroupElement) FromAffine(x, y *big.Int) *ProjectiveGroupElement {
	v.X.FromBig(x)
	v.Y.FromBig(y)
	v.Z.One()
	return v
}

func (v *ProjectiveGroupElement) ToAffine() (*big.Int, *big.Int) {
	var x, y, zinv radix51.FieldElement

	zinv.Invert(&v.Z)
	x.Mul(&v.X, &zinv)
	y.Mul(&v.Y, &zinv)

	return x.ToBig(), y.ToBig()
}

// HWCD Section 3: "Given (X : Y : Z) in [projective coordinates] passing to
// [extended coordinates, (X : Y : T : Z)] can be performed in 3M+1S by computing
// (XZ, YZ, XY, Z^2)"
func (v *ProjectiveGroupElement) ToExtended(r *ExtendedGroupElement) {
	r.X.Mul(&v.X, &v.Z)
	r.Y.Mul(&v.Y, &v.Z)
	r.T.Mul(&v.X, &v.Y)
	r.Z.Square(&v.Z)
}

func (v *ProjectiveGroupElement) Zero() *ProjectiveGroupElement {
	v.X.Zero()
	v.Y.One()
	v.Z.One()
	return v
}

// Because we are often converting from affine, we can use "mdbl-2008-bbjlp"
// which assumes Z1=1. We also assume a = -1.
//
// Assumptions: Z1 = 1.
// Cost: 2M + 4S + 1*a + 7add + 1*2.
// Source: 2008 BernsteinBirknerJoyeLangePeters
//         http://eprint.iacr.org/2008/013, plus Z1=1, plus standard simplification.
// Explicit formulas:
//
//       B = (X1+Y1)^2
//       C = X1^2
//       D = Y1^2
//       E = a*C
//       F = E+D
//       X3 = (B-C-D)*(F-2)
//       Y3 = F*(E-D)
//       Z3 = F^2-2*F
//
// This assumption is one reason why this package is internal. For instance, it
// will not hold throughout a Montgomery ladder, when we convert to projective
// from possibly arbitrary extended coordinates.
func (v *ProjectiveGroupElement) DoubleZ1(u *ProjectiveGroupElement) *ProjectiveGroupElement {
	var B, C, D, E, F radix51.FieldElement

	if u.Z.Equal(radix51.One) != 1 {
		panic("ed25519: DoubleZ1 called with Z != 1")
	}

	B.Square(B.Add(&u.X, &u.Y)) // B = (X1+Y1)^2
	C.Square(&u.X)              // C = X1^2
	D.Square(&u.Y)              // D = Y1^2
	E.Neg(&C)                   // E = a*C where a = -1
	F.Add(&E, &D)               // F = E + D

	// X3 = (B-C-D)*(F-2)
	v.Y.Sub(v.Y.Sub(&B, &C), &D)
	v.X.Mul(&v.Y, v.X.Sub(&F, radix51.Two))

	// Y3 = F*(E-D)
	v.Y.Mul(&F, v.Y.Sub(&E, &D))

	// Z3 = F^2 - 2*F
	v.Z.Square(&F)
	v.Z.Sub(&v.Z, &F)
	v.Z.Sub(&v.Z, &F)

	return v
}

// IsOnCurve reports whether the given affine coordinate (x,y) lies on the curve
// by checking that -x^2 + y^2 - 1 - dx^2y^2 = 0 (mod p).
func IsOnCurve(x, y *radix51.FieldElement) bool {
	var lh, y2, rh radix51.FieldElement
	lh.Square(x)             // x^2
	y2.Square(y)             // y^2
	rh.Mul(&lh, &y2)         // x^2*y^2
	rh.Mul(&rh, D)           // d*x^2*y^2
	rh.Add(&rh, radix51.One) // 1 + d*x^2*y^2
	lh.Neg(&lh)              // -x^2
	lh.Add(&lh, &y2)         // -x^2 + y^2
	lh.Sub(&lh, &rh)         // -x^2 + y^2 - 1 - dx^2y^2

	return lh.Equal(radix51.Zero) == 1
}
