// * Copyright 2003-2005 Colin Percival
// * All rights reserved
// *
// * Redistribution and use in source and binary forms, with or without
// * modification, are permitted providing that the following conditions
// * are met:
// * 1. Redistributions of source code must retain the above copyright
// *    notice, this list of conditions and the following disclaimer.
// * 2. Redistributions in binary form must reproduce the above copyright
// *    notice, this list of conditions and the following disclaimer in the
// *    documentation and/or other materials provided with the distribution.
// *
// * THIS SOFTWARE IS PROVIDED BY THE AUTHOR ``AS IS'' AND ANY EXPRESS OR
// * IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
// * WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
// * ARE DISCLAIMED.  IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY
// * DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
// * DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS
// * OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION)
// * HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT,
// * STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING
// * IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
// * POSSIBILITY OF SUCH DAMAGE.

// Package bsdiff is a binary diff program using suffix sorting.
package bsdiff

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/dsnet/compress/bzip2"
	"github.com/kiteco/go-bsdiff/pkg/util"
)

// Bytes takes the old and new byte slices and outputs the diff
func Bytes(oldbs, newbs []byte) ([]byte, error) {
	return diffb(oldbs, newbs)
}

// Reader takes the old and new binaries and outputs to a stream of the diff file
func Reader(oldbin io.Reader, newbin io.Reader, patchf io.Writer) error {
	oldbs, err := ioutil.ReadAll(oldbin)
	if err != nil {
		return err
	}
	newbs, err := ioutil.ReadAll(newbin)
	if err != nil {
		return err
	}
	diffbytes, err := diffb(oldbs, newbs)
	if err != nil {
		return err
	}
	return util.PutWriter(patchf, diffbytes)
}

// File reads the old and new files to create a diff patch file
func File(oldfile, newfile, patchfile string) error {
	oldbs, err := ioutil.ReadFile(oldfile)
	if err != nil {
		return fmt.Errorf("could not read oldfile '%v': %v", oldfile, err.Error())
	}
	newbs, err := ioutil.ReadFile(newfile)
	if err != nil {
		return fmt.Errorf("could not read newfile '%v': %v", newfile, err.Error())
	}
	diffbytes, err := diffb(oldbs, newbs)
	if err != nil {
		return fmt.Errorf("bsdiff: %v", err.Error())
	}
	if err := ioutil.WriteFile(patchfile, diffbytes, 0644); err != nil {
		return fmt.Errorf("could create patchfile '%v': %v", patchfile, err.Error())
	}
	return nil
}

func diffb(oldbin, newbin []byte) ([]byte, error) {
	// File format:
	// --- header ---
	//  0     -  7       : "BSDIFF40"
	//  8     - 15       : X
	// 16     - 23       : Y
	// 24     - 31       : len(newfile)
	// 32     - 63       : sha256sum(oldfile)
	// ---  data  ---
	// 64     - 64+X-1   : bzip2(control block)
	// 64+X   - 64+X+Y-1 : bzip2(diff block)
	// 64+X+Y - ??       : bzip2(extra block)

	const headerLen = 64

	bziprule := &bzip2.WriterConfig{
		Level: bzip2.BestCompression,
	}
	iii := make([]int, len(oldbin)+1)
	qsufsort(iii, oldbin)

	//var db
	var dblen, eblen int

	// create the patch file
	pf := new(util.BufWriter)
	buf := make([]byte, 8)

	newsize := len(newbin)
	oldsize := len(oldbin)

	// - header
	// total length is 64, but the checksum (32 bytes) appends to the slice,
	header := make([]byte, 32, headerLen)
	copy(header, []byte("BSDIFF40"))
	offtout(0, header[8:])
	offtout(0, header[16:])
	offtout(newsize, header[24:])

	sum := sha256.New()
	if _, err := sum.Write(oldbin); err != nil {
		return nil, err
	}
	if sum.Size() != 32 {
		panic("unexpected: sha256sum size is not 32 bytes")
	}
	header = sum.Sum(header) // appends to header

	if _, err := pf.Write(header); err != nil {
		return nil, err
	}
	// Compute the differences, writing ctrl as we go
	pfbz2, err := bzip2.NewWriter(pf, bziprule)
	if err != nil {
		return nil, err
	}
	var scan, ln, lastscan, lastpos, lastoffset int

	var oldscore, scsc int
	var pos int

	var s, Sf, lenf, Sb, lenb int
	var overlap, Ss, lens int

	db := make([]byte, newsize+1)
	eb := make([]byte, newsize+1)

	defer func() {
		if pfbz2 != nil {
			pfbz2.Close()
		}
	}()

	for scan < newsize {
		oldscore = 0

		// scsc = scan += len
		scan += ln
		scsc = scan
		for scan < newsize {
			ln = search(iii, oldbin, newbin[scan:], 0, oldsize, &pos)

			for scsc < scan+ln {
				if scsc+lastoffset < oldsize && oldbin[scsc+lastoffset] == newbin[scsc] {
					oldscore++
				}
				scsc++
			}
			if ln == oldscore && ln != 0 {
				break
			}
			if ln > oldscore+8 {
				break
			}
			if scan+lastoffset < oldsize && oldbin[scan+lastoffset] == newbin[scan] {
				oldscore--
			}
			//
			scan++
		}

		if ln != oldscore || scan == newsize {
			s = 0
			Sf = 0
			lenf = 0
			i := 0
			for lastscan+i < scan && lastpos+i < oldsize {
				if oldbin[lastpos+i] == newbin[lastscan+i] {
					s++
				}
				i++
				if s*2-i > Sf*2-lenf {
					Sf = s
					lenf = i
				}
			}

			lenb = 0
			if scan < newsize {
				s = 0
				Sb = 0
				for i = 1; scan >= lastscan+i && pos >= i; i++ {
					if oldbin[pos-i] == newbin[scan-i] {
						s++
					}
					if s*2-i > Sb*2-lenb {
						Sb = s
						lenb = i
					}
				}
			}

			if lastscan+lenf > scan-lenb {
				overlap = (lastscan + lenf) - (scan - lenb)
				s = 0
				Ss = 0
				lens = 0
				for i = 0; i < overlap; i++ {
					if newbin[lastscan+lenf-overlap+i] == oldbin[lastpos+lenf-overlap+i] {
						s++
					}

					if newbin[scan-lenb+i] == oldbin[pos-lenb+i] {
						s--
					}
					if s > Ss {
						Ss = s
						lens = i + 1
					}
				}

				lenf += lens - overlap
				lenb -= lens
			}

			for i = 0; i < lenf; i++ {
				db[dblen+i] = newbin[lastscan+i] - oldbin[lastpos+i]
			}
			for i = 0; i < (scan-lenb)-(lastscan+lenf); i++ {
				eb[eblen+i] = newbin[lastscan+lenf+i]
			}

			dblen += lenf
			eblen += (scan - lenb) - (lastscan + lenf)

			offtout(lenf, buf)
			if _, err := pfbz2.Write(buf); err != nil {
				return nil, err
			}

			offtout((scan-lenb)-(lastscan+lenf), buf)
			if _, err := pfbz2.Write(buf); err != nil {
				return nil, err
			}

			offtout((pos-lenb)-(lastpos+lenf), buf)
			if _, err := pfbz2.Write(buf); err != nil {
				return nil, err
			}

			lastscan = scan - lenb
			lastpos = pos - lenb
			lastoffset = pos - scan
		}
	}
	if err = pfbz2.Close(); err != nil {
		return nil, err
	}

	// Compute size of compressed ctrl data
	ln = pf.Len()
	offtout(ln-headerLen, header[8:])

	// Write compressed diff data
	pfbz2, err = bzip2.NewWriter(pf, bziprule)
	if err != nil {
		return nil, err
	}
	if _, err = pfbz2.Write(db[:dblen]); err != nil {
		return nil, err
	}

	if err = pfbz2.Close(); err != nil {
		return nil, err
	}
	// Compute size of compressed diff data
	newsize = pf.Len()
	offtout(newsize-ln, header[16:])
	// Write compressed extra data
	pfbz2, err = bzip2.NewWriter(pf, bziprule)
	if err != nil {
		return nil, err
	}
	if _, err = pfbz2.Write(eb[:eblen]); err != nil {
		return nil, err
	}
	if err = pfbz2.Close(); err != nil {
		return nil, err
	}
	// Seek to the beginning, write the header, and close the file
	if _, err = pf.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	if _, err = pf.Write(header); err != nil {
		return nil, err
	}

	return pf.Bytes(), nil
}

func search(iii []int, oldbin []byte, newbin []byte, st, en int, pos *int) int {
	var x, y int
	oldsize := len(oldbin)
	newsize := len(newbin)

	if en-st < 2 {
		x = matchlen(oldbin[iii[st]:], newbin)
		y = matchlen(oldbin[iii[en]:], newbin)

		if x > y {
			*pos = iii[st]
			return x
		}
		*pos = iii[en]
		return y
	}

	x = st + (en-st)/2
	cmpln := util.Min(oldsize-iii[x], newsize)
	if bytes.Compare(oldbin[iii[x]:iii[x]+cmpln], newbin[:cmpln]) < 0 {
		return search(iii, oldbin, newbin, x, en, pos)
	}
	return search(iii, oldbin, newbin, st, x, pos)
}

func matchlen(oldbin []byte, newbin []byte) int {
	var i int
	oldsize := len(oldbin)
	newsize := len(newbin)
	for (i < oldsize) && (i < newsize) {
		if oldbin[i] != newbin[i] {
			break
		}
		i++
	}
	return i
}

// offtout puts an int64 (little endian) to buf
func offtout(x int, buf []byte) {
	var y int
	if x < 0 {
		y = -x
	} else {
		y = x
	}
	buf[0] = byte(y % 256)
	y -= int(buf[0])
	y = y / 256
	buf[1] = byte(y % 256)
	y -= int(buf[1])
	y = y / 256
	buf[2] = byte(y % 256)
	y -= int(buf[2])
	y = y / 256
	buf[3] = byte(y % 256)
	y -= int(buf[3])
	y = y / 256
	buf[4] = byte(y % 256)
	y -= int(buf[4])
	y = y / 256
	buf[5] = byte(y % 256)
	y -= int(buf[5])
	y = y / 256
	buf[6] = byte(y % 256)
	y -= int(buf[6])
	y = y / 256
	buf[7] = byte(y % 256)

	if x < 0 {
		buf[7] |= 0x80
	}
}

func qsufsort(iii []int, buf []byte) {
	buckets := make([]int, 256)
	vvv := make([]int, len(iii))
	var i, h, ln int
	bufzise := len(buf)

	// -- Phase 0 --
	// Bucket sort (into iii) the natural numbers i = 0, ..., len(buf) based on the value buf[i].

	// count number of occurrences of each byte value
	for i = 0; i < bufzise; i++ {
		buckets[buf[i]]++
	}

	// prefix sum of buckets -> offsets into iii where each bucket starts
	for i = 1; i < 256; i++ {
		buckets[i] += buckets[i-1]
	}
	for i = 255; i > 0; i-- {
		buckets[i] = buckets[i-1]
	}
	buckets[0] = 0

	// populate iii based on bucket offsets
	for i = 0; i < bufzise; i++ {
		buckets[buf[i]]++
		iii[buckets[buf[i]]] = i
	}
	iii[0] = bufzise

	// vvv is the inverse of iii: for each index i = 0, ..., len(buf), store its sort ordering in iii.
	for i = 0; i < bufzise; i++ {
		vvv[i] = buckets[buf[i]]
	}
	vvv[bufzise] = 0

	// At this point, iii contains the "1-order" of buf.
	// The "h-order" for h>0 is the ordering of
	//   - the substrings of buf of length exactly h, and
	//   - the suffixes of buf of length less than h
	// Note that there are exactly n non-empty strings in the h-order for all h,
	// uniquely identified by their start offset in buf.

	// Next, we track groups of consecutive suffixes already in their final positions by
	// storing -x at the first such suffix in each group, where x in the size of the group.
	// This is non-destructive, since we still have vvv, the inverse of iii.

	// Currently, if a bucket has size 1, it's already in its final suffix-sort order.
	// Otherwise, it may not be. So store -1 at all size-1 bucket positions.
	// Note that groups are not yet "coalesced," so multiple groups may be adjacent.
	for i = 1; i < 256; i++ {
		if buckets[i] == buckets[i-1]+1 {
			iii[buckets[i]] = -1
		}
	}
	iii[0] = -1

	// -- Phase 1 --
	// Repeatedly double h, maintaining vvv = the inverse of the h-order of buf.
	// Coalesce groups of suffixes in their final positions, indicated by negative values in iii.
	for h = 1; iii[0] != -(bufzise + 1); h += h {
		ln = 0 // used to track the size of the coalesced group

		i = 0
		for i < bufzise+1 {
			if iii[i] < 0 {
				// at a group of suffixes in their final positions, skip it
				ln -= iii[i]
				i -= iii[i]
			} else {
				if ln != 0 {
					// coalesce all the groups we just saw into one.
					iii[i-ln] = -ln
				}
				// TODO(naman) when would this not be 1?
				ln = vvv[iii[i]] + 1 - i
				split(iii, vvv, i, ln, h)
				i += ln
				ln = 0
			}
		}
		if ln != 0 {
			iii[i-ln] = -ln
		}
	}

	for i = 0; i < bufzise+1; i++ {
		iii[vvv[i]] = i
	}
}

func split(iii, vvv []int, start, ln, h int) {
	var i, j, k, x int

	if ln < 16 {
		for k = start; k < start+ln; k += j {
			j = 1
			x = vvv[iii[k]+h]
			for i = 1; k+i < start+ln; i++ {
				if vvv[iii[k+i]+h] < x {
					x = vvv[iii[k+i]+h]
					j = 0
				}
				if vvv[iii[k+i]+h] == x {
					iii[k+j], iii[k+i] = iii[k+i], iii[k+j]
					j++
				}
			}
			for i = 0; i < j; i++ {
				vvv[iii[k+i]] = k + j - 1
			}
			if j == 1 {
				iii[k] = -1
			}
		}
		return
	}

	x = vvv[iii[start+(ln/2)]+h]
	var jj, kk int
	for i = start; i < start+ln; i++ {
		if vvv[iii[i]+h] < x {
			jj++
		} else if vvv[iii[i]+h] == x {
			kk++
		}
	}
	jj += start
	kk += jj

	i = start
	j = 0
	k = 0
	for i < jj {
		if vvv[iii[i]+h] < x {
			i++
		} else if vvv[iii[i]+h] == x {
			iii[i], iii[jj+j] = iii[jj+j], iii[i]
			j++
		} else {
			iii[i], iii[kk+k] = iii[kk+k], iii[i]
			k++
		}
	}
	for jj+j < kk {
		if vvv[iii[jj+j]+h] == x {
			j++
		} else {
			iii[jj+j], iii[kk+k] = iii[kk+k], iii[jj+j]
			k++
		}
	}
	if jj > start {
		split(iii, vvv, start, jj-start, h)
	}

	for i = 0; i < kk-jj; i++ {
		vvv[iii[jj+i]] = kk - 1
	}
	if jj == kk-1 {
		iii[jj] = -1
	}

	if start+ln > kk {
		split(iii, vvv, kk, start+ln-kk, h)
	}
}
