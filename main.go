package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"os"
)

const PTR byte = 1
const BYTE byte = 0

type InMemoryOutput struct {
	Headers              []byte
	currentHeaderByteLen int
	Data                 []byte
}

func NewInMemoryOutput() InMemoryOutput {
	return InMemoryOutput{
		Headers: make([]byte, 1),
	}
}

func (imo *InMemoryOutput) AddByteInHeader(kind byte) {
	if imo.currentHeaderByteLen > 7 {
		imo.Headers = append(imo.Headers, 0)
		imo.currentHeaderByteLen = 0
	}

	imo.Headers[len(imo.Headers)-1] |= (kind << byte(imo.currentHeaderByteLen))
	imo.currentHeaderByteLen++
}

const BUF_CAP = 4096

type Buffer struct {
	Imo    InMemoryOutput
	Buffer [BUF_CAP]byte
	Len    int
}

func NewBuffer() Buffer {
	return Buffer{
		Imo: NewInMemoryOutput(),
	}
}

func (b *Buffer) WriteByte(c byte) {
	b.Imo.AddByteInHeader(BYTE)
	if b.Len >= BUF_CAP {
		b.Imo.Data = append(b.Imo.Data, b.Buffer[:BUF_CAP]...)
		b.Len = 0
	}

	b.Buffer[b.Len] = c
	b.Len++
}

func (b *Buffer) WritePtr(ptr, size byte) {
	b.Imo.AddByteInHeader(PTR)

	if b.Len+2 > BUF_CAP {
		b.Imo.Data = append(b.Imo.Data, b.Buffer[:BUF_CAP]...)
		b.Len = 0
	}

	b.Buffer[b.Len] = ptr
	b.Len++
	b.Buffer[b.Len] = size
	b.Len++
}

func (b Buffer) Flush() InMemoryOutput {
	b.Imo.Data = append(b.Imo.Data, b.Buffer[:b.Len]...)

	return b.Imo
}

type Input struct {
	Data      []byte
	LookAhead []byte
}

func NewInputFromFile(filename string) Input {
	var i Input

	data, err := ioutil.ReadFile(filename)
	check(err)

	i.Data = data

	return i
}

func (i *Input) FillLookAhead() {
	if i.Data == nil {
		panic("i.Data is nil")
	}

	if len(i.Data) < 16 {
		i.LookAhead = i.Data
		return
	}

	i.LookAhead = i.Data[:16]
}

func Compare(l, r []byte) bool {
	if l == nil || r == nil {
		return false
	} else if len(l) > len(r) {
		return false
	}

	for i := range l {
		if l[i] != r[i] {
			return false
		}
	}

	return true
}

func (in *Input) Match(buf *Buffer) (bool, []byte, int, int) {
	for i := len(in.LookAhead); i >= 0; i-- {
		endOff := in.LookAhead[:i]
		for k := 0; k < len(buf.Buffer); k++ {
			if Compare(endOff, buf.Buffer[k:]) {
				if len(endOff) < 3 {
					return false, nil, 0, 0
				}
				return true, endOff, buf.Len - k, len(endOff)
			}
		}
	}

	return false, nil, 0, 0
}

func Loop(in *Input, buf *Buffer) InMemoryOutput {
	for len(in.Data) != 0 {
		in.FillLookAhead()
		if len(in.LookAhead) == 0 {
			break
		}
		matched, _, ptr, size := in.Match(buf)
		if matched {
			first, second := pack(ptr, size)
			buf.WritePtr(first, second)
			in.Data = in.Data[size:]
		} else {
			buf.WriteByte(in.LookAhead[0])
			in.Data = in.Data[1:]
		}
	}

	return buf.Flush()
}

// subtracting values by 1, u12 numbers range from 0 - 4095, we want to start from 1
func pack(ptr, size int) (byte, byte) {
	ptr--
	size--
	first := byte(ptr >> 4)
	second := byte(((ptr & 15) << 4) | size)

	return first, second
}

func unpack(first, second byte) (int, int) {
	newfirst := (int(first) << 4) + int(second>>4)
	newsecond := int(second & 15)

	return newfirst + 1, newsecond + 1
}

func (in *InMemoryOutput) ToFile(filename string) {
	f, err := os.Create("out.lzss")
	check(err)
	defer f.Close()

	enc := gob.NewEncoder(f)
	enc.Encode(in)
}

type Queue struct {
	Data []byte
}

func NewQueue(cap int) Queue {
	var q Queue

	q.Data = make([]byte, 0, cap)

	return q
}

func (q *Queue) Push(s []byte) {
	q.Data = bytes.Join([][]byte{q.Data, s}, []byte{})
}

func DecompressFromFileToFile(filename string) {
	f, err := os.Open(filename + ".lzss")
	check(err)
	defer f.Close()

	var imo InMemoryOutput
	decoder := gob.NewDecoder(f)
	decoder.Decode(&imo)

	pos := 0

	queue := NewQueue(len(imo.Data))

	for h := 0; h < len(imo.Headers)-1; h++ {
		rename(&imo, &queue, &pos, imo.Headers[h], 8)
	}

	rename(&imo, &queue, &pos, imo.Headers[len(imo.Headers)-1], len(imo.Data)-pos)

	out, err := os.Create(filename)
	check(err)
	defer out.Close()

	out.Write(queue.Data)
}

func rename(imo *InMemoryOutput, queue *Queue, pos *int, h byte, cols int) {
	for i := 0; *pos < len(imo.Data) && i < cols; i++ {
		if h&(1<<i) != 0 {
			ptr, size := unpack(imo.Data[*pos], imo.Data[*pos+1])

			len_adjusted := imo.Data[*pos-ptr:][:size]

			queue.Push(len_adjusted)
			*pos += 2
		} else {
			queue.Push([]byte{imo.Data[*pos]})
			*pos++
		}
	}
}

func Compress() {
	b := NewBuffer()
	i := NewInputFromFile("other")

	imo := Loop(&i, &b)

	imo.ToFile("out.lzss")

}

func testpacking(one, two int) {
	x, y := pack(one, two)
	q, w := unpack(x, y)

	if q != one || w != two {
		fmt.Println("failed", one, two, x, y, q, w)
	}
}

func main() {
	testpacking(4096, 16)
	testpacking(2600, 16)
	testpacking(1, 1)
	testpacking(34, 16)

	Compress()

	DecompressFromFileToFile("out")
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}
