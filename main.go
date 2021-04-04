package main

import (
	"fmt"
	"os"
	"io/ioutil"
	"strings"
)

type Unit interface {
	Bytes() []byte
}
	
type Char struct {
	v byte
}

// members are decremented upon compression and incremented after decompressed 
type Ptr struct {
	back uint 
	len byte
}	

type CharStr struct {
	index uint
	str []Char
}

func (c Char) Bytes() []byte {
	return []byte{c.v}
}

func (c Ptr) Bytes() []byte {
	c.back--
	c.len--
	one := byte(c.back >> 4)
	two := byte((c.back & 15) << 4)
	return []byte{one, two | c.len}
}

const WINDOW_SIZE_IN_BYTES = 4096

var buffer []Unit
var window []Unit

var number_of_headers uint64
var headers []byte
var data []byte

func main() {
	handle_command_line_args()
}

func handle_command_line_args() {
	args := os.Args

	if len(args) < 3 {
		fmt.Println("Usage: lzss [-c or -d] [FILE]")
		os.Exit(0)
	}

	filename := args[2]
	
	bytes, err := ioutil.ReadFile(filename)
	panifer(err)
		
	if args[1] == "-d" {
		if !strings.Contains(filename, ".lzss") {
			fmt.Printf("file \"%s\" doesn't have .lzss extension \n")
			return
		}
		
		output := decompress(bytes)

		out, err := os.Create(filename[:len(".lzss")] + ".out")
		panifer(err)

		out.Write(output)
	} else if args[1] == "-c" {
		bytes, err := ioutil.ReadFile(filename)
		panifer(err)
		
		compress(filename + ".lzss", bytes)
	} else {
		fmt.Println("Usage: lzss [-c or -d] [FILE]")
	}
}

func compress(output_filename string, original []byte) {
	input := original
	
	for len(input) != 0 {
		window = window_units_at_back_of_array(buffer)

		cstrings := find_charstrs_in_window(window)

		snippet := take16(input)
		match, ptr := string_search(snippet, cstrings)
		
		if match == true && ptr.(Ptr).len == 2 {
			add_string_to_buffer(snippet[:2])
			input = input[2:]
		} else if match == true {
			v, ok := ptr.(Ptr)
			if !ok {
				panic("Supposed to be Ptr!")
			}
			input = input[v.len:]
			buffer = append(buffer, v)
		} else {
			add_string_to_buffer(snippet[:1])
			input = input[1:]
		}
	}
	
	encode_from_buffer()

	f := create_file(output_filename)
	defer func() {
		panifer(f.Close())
	}()
	
	write_to_file(f)
}

func decompress(data []byte) (ret []byte) {
	number_of_headers, data = bytes_to_uint64(data)
	headers = data[:number_of_headers]
	data = data[number_of_headers:]

	ret = make([]byte, 0, len(data) / 2)
	index := 0
	for _, h := range headers {
		for i := 0; i < 8 && index < len(data); i++ {
			is_ptr_flag := (h & (1 << i) != 0)
			if is_ptr_flag == true {
				back, l := unpack_ptr(data[index:])
				str_start := index - int(back) 
				str := data[str_start : str_start + int(l)]
				for _, v := range str {
					ret = append(ret, v)
				}
				index += 2
			} else {
				ret = append(ret, data[index])
				index++
			}
		}
	}

	return ret
}

func unpack_ptr(d []byte) (back byte, l byte) {
	d = d[:2]

	back = (d[0] << 4) + (d[1] >> 4)
	l = d[1] & 15

	return back + 1 , l + 1
}

func uint64_to_bytes(i uint64) (r []byte) {
	r = make([]byte, 8)

	index := 0
	for shift_by := 0; shift_by < 64; shift_by += 8 {
		r[index] = byte(i >> shift_by)
		index++
	}

	return r
}

func bytes_to_uint64(b []byte) (ri uint64, rb []byte) {
	rb = b[8:]
	b = b[:8]

	for i := 0; i < 8; i++ {
		ri += ( uint64(b[i]) << (i * 8) )
	}

	return ri, rb
}

func write_to_file(f *os.File) int {
	w, err := f.Write(uint64_to_bytes(number_of_headers))
	panifer(err)

	w2, err := f.Write(headers)
	panifer(err)

	w3, err := f.Write(data)
	panifer(err)

	return w + w2 + w3
}

func add_string_to_buffer(input []byte) {
	for _, c := range input {
		buffer = append(buffer, Char{c})
	}
}

func take16(s []byte) []byte {
	if len(s) < 16 {
		return s
	}

	return s[:16]
}

func window_units_at_back_of_array(units []Unit) []Unit {
	bytes := 0

	start := len(units)
	
	for i := len(units) - 1; i >= 0; i-- {
		head := units[i]	
		
		switch head.(type) {
		case Char:
			bytes += 1
		case Ptr:
			bytes += 2
		default:
			panic("Impossible State!")
		}

		if bytes > WINDOW_SIZE_IN_BYTES {
			return units[start:]
		}
		start--
	}
	
	return units[start:]
}

func find_charstrs_in_window(window []Unit) (r []CharStr) {
	for i := 0; i < len(window); i++ {		
		switch v := window[i].(type) {
		case Char:
			var s CharStr
			s.index = uint(i)
			s.str = make([]Char, 0, 16)
			s.str = append(s.str, v)
			
			for i++; i < len(window); i++ {
				switch v := window[i].(type) {
				case Char:
					s.str = append(s.str, v)
					continue
				}

				break
			}

			r = append(r, s)
		}
	}

	return r
}

func strcmp(chars []byte, cs CharStr) (bool, CharStr) {
	if len(chars) == 1 || len(cs.str) == 1 {
		return false, CharStr{}
	} else if len(chars) > len(cs.str) {
		return false, CharStr{}
	}
		
	for i := 0; i < len(chars); i++ {
		if chars[i] != cs.str[i].v {
			return false, cs
		}
	}
	return true, cs 
}

func string_search(chars []byte, strs []CharStr) (bool, Unit) {
	if len(chars) == 0 {
		return false, Ptr{}
	}

	for ; len(chars) > 0; chars = chars[:len(chars)-1] {
		for _, v := range strs {
			for j := 0; j < len(v.str); j++ {
				l := v
				l.str = v.str[j:]
				if b, cs := strcmp(chars, l); b {
					return true, Ptr{window_len_in_bytes(window[cs.index + uint(j):]), byte(len(chars))}
				}
			}
		}
	}
	
	return false, Ptr{}
}

func window_len_in_bytes(window []Unit) (r uint) {
	for i := 0; i < len(window); i++ {
		switch window[i].(type) {
		case Char:
			r++
		case Ptr:
			r += 2
		}
	}

	return r
}

func encode_from_buffer() {
	for len(buffer) > 0 {
		var h byte 
		for i := 0; i < 8 && len(buffer) > 0; i++ {
			var t byte = 0
			head := buffer[0]
			
			switch head.(type) {
			case Ptr:
				t = 1				
			}
			h |= (t << i)

			bytes := head.Bytes()
			for _, v := range bytes {
				data = append(data, v)
			}
			buffer = buffer[1:]
		}

		number_of_headers++
		headers = append(headers, h)
	}
}

func create_file(name string) *os.File {
	file, err := os.Create(name)
	panifer(err)

	return file
}

func panifer(e error) {
	if e != nil {
		panic(e)
	}
}
