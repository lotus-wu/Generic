package spn

import (
	"crypto/rand"

	"github.com/OpenWhiteBox/primitives/encoding"
	"github.com/OpenWhiteBox/primitives/gfmatrix"
	"github.com/OpenWhiteBox/primitives/number"
)

// incrementalMatrices implements succint operations over a slice of incremental matrices.
type incrementalMatrices []gfmatrix.IncrementalMatrix

// NewIncrementalMatrices returns a new slice of x n-by-n incremental matrices.
func newIncrementalMatrices(x, n int) (ims incrementalMatrices) {
	ims = make([]gfmatrix.IncrementalMatrix, x)
	for i, _ := range ims {
		ims[i] = gfmatrix.NewIncrementalMatrix(n)
	}

	return
}

// SufficientlyDefined returns true if every incremental matrix is sufficiently defined. The must all have a
// 9-dimensional nullspace or smallter. This way, it is small enough to search, but not so small that we have nowhere to
// look for solutions.
func (ims incrementalMatrices) SufficientlyDefined() bool {
	for _, im := range ims {
		if im.Len() < 247 {
			return false
		}
	}

	return true
}

// Matrices returns a slice of matrices, one for each incremental matrix.
func (ims incrementalMatrices) Matrices() (out []gfmatrix.Matrix) {
	out = make([]gfmatrix.Matrix, len(ims))
	for i, im := range ims {
		out[i] = im.Matrix()
	}

	return out
}

// randomLinearCombination returns a random linear combination of a set of basis vectors.
func randomLinearCombination(basis []gfmatrix.Row) gfmatrix.Row {
	coeffs := make([]byte, len(basis))
	rand.Read(coeffs)

	v := gfmatrix.NewRow(basis[0].Size())

	for i, c_i := range coeffs {
		v = v.Add(basis[i].ScalarMul(number.ByteFieldElem(c_i)))
	}

	return v
}

// findPermutation takes a set of vectors and finds a linear combination of them that gives a permutation vector.
func findPermutation(basis []gfmatrix.Row) gfmatrix.Row {
	for true {
		v := randomLinearCombination(basis)

		if v[:256].IsPermutation() {
			return v
		}
	}

	return nil
}

// newSBox takes a permutation vector as input and returns its corresponding S-Box. It inverts the S-Box if backwards is
// true (because the permutation vector we found was for the inverse S-box).
func newSBox(v gfmatrix.Row, backwards bool) (out encoding.SBox) {
	for i, v_i := range v[0:256] {
		out.EncKey[i] = byte(v_i)
	}

	for i, j := range out.EncKey {
		out.DecKey[j] = byte(i)
	}

	if backwards { // Reverse EncKey and DecKey if we recover S^-1
		out.EncKey, out.DecKey = out.DecKey, out.EncKey
	}

	return
}

// RecoverSBoxes implements a specific variant of the Cube attack to remove the trailing S-box layer of the given
// cipher. It uses the plaintexts generated by generator.
func RecoverSBoxes(cipher encoding.Block, generator func() [][16]byte) (last encoding.ConcatenatedBlock, rest encoding.Block) {
	ims := newIncrementalMatrices(16, 256)

	for attempt := 0; attempt < 2000 && !ims.SufficientlyDefined(); attempt++ {
		pts := generator()
		cts := make([][16]byte, len(pts))

		for i, pt := range pts {
			cts[i] = cipher.Encode(pt)
		}

		for pos := 0; pos < 16; pos++ {
			row := gfmatrix.NewRow(256)

			for _, ct := range cts {
				row[ct[pos]] = row[ct[pos]].Add(0x01)
			}

			ims[pos].Add(row)
		}
	}

	if !ims.SufficientlyDefined() {
		panic("Cube attack failed to find enough linear relations in the S-boxes.")
	}

	for pos, m := range ims.Matrices() {
		last[pos] = newSBox(findPermutation(m.NullSpace()), true)
	}

	return last, encoding.ComposedBlocks{cipher, encoding.InverseBlock{last}}
}
