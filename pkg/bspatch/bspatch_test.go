package bspatch

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/kiteco/go-bsdiff/pkg/util"
)

var (
	oldfile = []byte{
		0x66, 0xFF, 0xD1, 0x55, 0x56, 0x10, 0x30, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xD1,
	}
	newfilecomp = []byte{
		0x66, 0xFF, 0xD1, 0x55, 0x56, 0x10, 0x30, 0x00,
		0x44, 0x45, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0xD1, 0xFF, 0xD1,
	}
	patchfile = []byte{
		0x42, 0x53, 0x44, 0x49, 0x46, 0x46, 0x34, 0x30,
		0x29, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x2A, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x13, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,

		0x71, 0x1c, 0x5e, 0xc8, 0xc0, 0x49, 0x99, 0xdd,
		0x34, 0x84, 0x81, 0x69, 0x74, 0x01, 0x01, 0xb6,
		0xbf, 0x12, 0x09, 0xf0, 0xed, 0xa3, 0xf9, 0xf0,
		0x98, 0x7e, 0x60, 0xa3, 0x59, 0x13, 0xb2, 0x95,

		0x42, 0x5A, 0x68, 0x39, 0x31, 0x41, 0x59, 0x26,
		0x53, 0x59, 0xDA, 0xE4, 0x46, 0xF2, 0x00, 0x00,
		0x05, 0xC0, 0x00, 0x4A, 0x09, 0x20, 0x00, 0x22,
		0x34, 0xD9, 0x06, 0x06, 0x4B, 0x21, 0xEE, 0x17,
		0x72, 0x45, 0x38, 0x50, 0x90, 0xDA, 0xE4, 0x46,
		0xF2, 0x42, 0x5A, 0x68, 0x39, 0x31, 0x41, 0x59,
		0x26, 0x53, 0x59, 0x30, 0x88, 0x1C, 0x89, 0x00,
		0x00, 0x02, 0xC4, 0x00, 0x44, 0x00, 0x06, 0x00,
		0x20, 0x00, 0x21, 0x21, 0xA0, 0xC3, 0x1B, 0x03,
		0x3C, 0x5D, 0xC9, 0x14, 0xE1, 0x42, 0x40, 0xC2,
		0x20, 0x72, 0x24, 0x42, 0x5A, 0x68, 0x39, 0x31,
		0x41, 0x59, 0x26, 0x53, 0x59, 0x65, 0x25, 0x30,
		0x43, 0x00, 0x00, 0x00, 0x40, 0x02, 0xC0, 0x00,
		0x20, 0x00, 0x00, 0x00, 0xA0, 0x00, 0x22, 0x1F,
		0xA4, 0x19, 0x82, 0x58, 0x5D, 0xC9, 0x14, 0xE1,
		0x42, 0x41, 0x94, 0x94, 0xC1, 0x0C,
	}
)

func TestPatch(t *testing.T) {
	tests := []struct {
		wbufsz  int
		cpbufsz int
	}{
		{50, 50},
		{19, 19},
		{1, 4},
		{4, 1},
		{2, 4},
		{4, 2},
		{3, 4},
		{4, 3},
		{4, 4},
		{7, 9},
		{9, 7},
	}

	for _, test := range tests {
		writeBufferSize = test.wbufsz
		copyBufferSize = test.cpbufsz

		desc := fmt.Sprintf("writeBufferSize: %v, copyBufferSize: %v", writeBufferSize, copyBufferSize)
		newfile, err := Bytes(oldfile, patchfile)
		if err != nil {
			t.Errorf("With %s failed with error: %s", desc, err.Error())
			continue
		}
		if !bytes.Equal(newfile, newfilecomp) {
			res := fmt.Sprintf("expected: %v, got: %v", newfilecomp, newfile)
			t.Errorf("With %s failed with unequal bytes %s", desc, res)
			continue
		}
	}
	// test invalid patch
	_, err := Bytes(oldfile, oldfile)
	if err == nil {
		t.Errorf("Invalid patch fail")
	}
}

func TestOfftin(t *testing.T) {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, 9001)
	n := offtin(buf)
	if n != 9001 {
		t.Fatal(n, "!=", 9001)
	}
}

func TestReader(t *testing.T) {
	oldrdr := bytes.NewReader(oldfile)
	prdr := bytes.NewReader(patchfile)
	newf := new(bytes.Buffer)
	if err := Reader(oldrdr, newf, prdr); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 8)
	newf.Read(buf)
	if !bytes.Equal(buf, []byte{0x66, 0xFF, 0xD1, 0x55, 0x56, 0x10, 0x30, 0x00}) {
		t.Fatal(buf)
	}
}

func TestFile(t *testing.T) {
	tf0, err := ioutil.TempFile(os.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	t0n := tf0.Name()
	tf1, err := ioutil.TempFile(os.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	t1n := tf1.Name()
	if err := util.PutWriter(tf0, oldfile); err != nil {
		tf0.Close()
		tf1.Close()
		os.Remove(t0n)
		os.Remove(t1n)
		t.Fatal(err)
	}
	if err := util.PutWriter(tf1, patchfile); err != nil {
		tf0.Close()
		tf1.Close()
		os.Remove(t0n)
		os.Remove(t1n)
		t.Fatal(err)
	}
	tf0.Close()
	tf1.Close()
	tp, err := ioutil.TempFile(os.TempDir(), "")
	if err != nil {
		os.Remove(t0n)
		os.Remove(t1n)
		t.Fatal(err)
	}
	tpp := tp.Name()
	tp.Close()
	if err := File(t0n, tpp, t1n); err != nil {
		os.Remove(t0n)
		os.Remove(t1n)
		os.Remove(tpp)
		t.Fatal(err)
	}
	os.Remove(t0n)
	os.Remove(t1n)
	os.Remove(tpp)
}

func TestFileErr(t *testing.T) {
	// oldfile err
	if err := File("__nil__", "__nil__", "__nil__"); err == nil {
		t.Fail()
	}
	tfl, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fail()
	}
	tfl.Write([]byte{10, 11, 12, 13, 14, 15, 16, 17})
	fn := tfl.Name()
	tfl.Close()
	defer func() {
		os.Remove(fn)
	}()
	if err := File(fn, "__nil__", "__nil__"); err == nil {
		t.Fail()
	}
	if err := File(fn, fn, "__nil__"); err == nil {
		t.Fail()
	}
	if err := File(fn, fn, fn); err == nil {
		t.Fail()
	}
}

type corruptReader int

func (r *corruptReader) Read(p []byte) (n int, err error) {
	return 0, fmt.Errorf("testing")
}

func TestReaderError(t *testing.T) {
	cr := corruptReader(0)
	b0 := bytes.NewReader([]byte{0, 0, 0, 0, 0, 1, 2, 3, 4, 5})
	b1 := new(bytes.Buffer)
	if err := Reader(&cr, b1, b0); err == nil {
		t.Fail()
	}
	if err := Reader(b0, b1, &cr); err == nil {
		t.Fail()
	}
}

func TestCorruptHeader(t *testing.T) {
	corruptPatch := []byte{
		0x41, 0x53, 0x44, 0x49, 0x46, 0x46, 0x34, 0x30,
		0x29, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x2A, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x13, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
	_, err := Bytes(corruptPatch, corruptPatch[:30])
	if err == nil {
		t.Fatal("header should be corrupt")
	}
	if _, ok := err.(*CorruptPatchError); ok {
		t.Fatal("header should be corrupt (2)")
	}
	_, err = Bytes(corruptPatch, corruptPatch)
	if err == nil {
		t.Fatal("header should be corrupt (3)")
	}
	if _, ok := err.(*CorruptPatchError); ok {
		t.Fatal("header should be corrupt (4)")
	}
	corruptPatch[0] = 0x42
	corruptLen := []byte{100, 0, 0, 0, 0, 0, 0, 128}
	copy(corruptPatch[8:], corruptLen)
	_, err = Bytes(corruptPatch, corruptPatch)
	if err == nil {
		t.Fatal("header should be corrupt (5)")
	}
	if _, ok := err.(*CorruptPatchError); ok {
		t.Fatal("header should be corrupt (6)")
	}
}

func TestInvalidChecksum(t *testing.T) {
	// copy patch file and change checksum byte
	mypatch := append(make([]byte, 0, len(patchfile)), patchfile...)
	mypatch[48] = 0

	_, err := Bytes(oldfile, mypatch)
	if err == nil {
		t.Errorf("checksum should not match")
	}
}

type lowcaprdr struct {
	read []byte
	n    int
}

func (r *lowcaprdr) Read(b []byte) (int, error) {
	if len(b) > 8 {
		copy(r.read[r.n:], b[:8])
		r.n += 8
		return 8, nil
	}
	copy(r.read[r.n:], b)
	r.n += len(b)
	return len(b), nil
}
