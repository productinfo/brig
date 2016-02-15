package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/golang/snappy"
)

// TODO: Tests schreiben (leere dateien, blockgröße -1, +0, +1 etc.)
// TODO: linter durchlaufen lassen.
// TODO: Sicherheitsprüfungen:
//       - prüfen ob index sortiert ist.
//       - prüfen ob blockSize > 0
// TODO: Seek.

type reader struct {
	rawR  io.ReadSeeker
	zipR  io.Reader
	index []Block
	// TODO: Noch benötigt?
	fileEndOff   int64
	tailBuf      []byte
	readBuf      *bytes.Buffer
	headerParsed bool
}

func (r *reader) Seek(offset int64, whence int) (int64, error) {
	return offset, nil
}

// TODO: Optimierung: Nutze binäre suche um korrekten index zu finden.
func (r *reader) blockLookup(currOff int64) (*Block, *Block) {
	var prevBlock, currBlock *Block
	for i, block := range r.index {
		currBlock = &r.index[i]
		if block.zipOff > currOff {
			return prevBlock, currBlock
		}
		prevBlock = &r.index[i]
	}
	return prevBlock, currBlock
}

func (r *reader) parseHeaderIfNeeded() error {
	if r.headerParsed {
		return nil
	}

	if _, err := r.rawR.Seek(-TailSize, os.SEEK_END); err != nil {
		return err
	}

	// Read size of tail.
	buf := [TailSize]byte{}
	if n, err := r.rawR.Read(buf[:]); err != nil || n != TailSize {
		return err
	}

	//TODO: Header parsing.
	tailSize := binary.LittleEndian.Uint64(buf[8:])
	r.tailBuf = make([]byte, tailSize)
	var err error
	seekIdx := -(int64(tailSize) + TailSize)
	if r.fileEndOff, err = r.rawR.Seek(seekIdx, os.SEEK_END); err != nil {
		fmt.Println(err)
		return err
	}

	if _, err := r.rawR.Read(r.tailBuf); err != nil {
		fmt.Println(err)
		return err
	}

	//Build Index
	for i := uint64(0); i < (tailSize / IndexBlockSize); i++ {
		b := Block{}
		b.unmarshal(r.tailBuf)
		fmt.Println("INDEX", b)
		r.index = append(r.index, b)
		r.tailBuf = r.tailBuf[IndexBlockSize:]
	}
	if _, err := r.rawR.Seek(0, os.SEEK_SET); err != nil {
		return err
	}
	r.headerParsed = true

	return nil
}

func (r *reader) Read(p []byte) (int, error) {
	if err := r.parseHeaderIfNeeded(); err != nil {
		fmt.Println(err)
		return 0, err
	}

	read := 0
	for {
		fmt.Println("READBUF LEN:", r.readBuf.Len())
		if r.readBuf.Len() != 0 {
			n := copy(p, r.readBuf.Next(len(p)))
			read += n
			p = p[n:]
			fmt.Println("P:", len(p))
		}
		if len(p) == 0 {
			break
		}

		if _, err := r.readBlockBuffered(); err != nil {
			fmt.Println("EOF?", read, err)
			return read, err
		}
	}
	fmt.Println("END:", read)
	return read, nil
}

func (r *reader) readBlockBuffered() (int64, error) {
	// Get current raw position
	curOff, err := r.rawR.Seek(0, os.SEEK_CUR)
	fmt.Println("CurrentOff:", curOff)
	if err != nil {
		return 0, err
	}

	// Get zip offset and set cursor to that position
	prevBlock, currBlock := r.blockLookup(curOff)
	fmt.Println("PREV AND CURR", prevBlock, currBlock)
	if currBlock == nil {
		return 0, io.EOF
	}

	currZipOff := prevBlock.zipOff
	fmt.Println("CURROFF:", currZipOff)
	if _, err = r.rawR.Seek(currZipOff, os.SEEK_SET); err != nil {
		fmt.Println(err)
		return 0, err
	}

	blockSize := currBlock.rawOff
	if prevBlock != nil {
		blockSize -= prevBlock.rawOff
	}
	if blockSize == 0 {
		return 0, io.EOF
	}

	return io.CopyN(r.readBuf, r.zipR, blockSize)
}

func NewReader(r io.ReadSeeker) io.ReadSeeker {
	return &reader{
		rawR:    r,
		zipR:    snappy.NewReader(r),
		readBuf: &bytes.Buffer{},
	}
}