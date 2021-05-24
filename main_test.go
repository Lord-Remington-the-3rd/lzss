package main

import (
	"fmt"
	"testing"
)

func TestPacking(t *testing.T) {
	f := func(one, two int) {
		x, y := pack(one, two)
		q, w := unpack(x, y)

		if q != one || w != two {
			fmt.Println("failed", one, two, x, y, q, w)
		}
	}

	f(4096, 16)
	f(2600, 16)
	f(1, 1)
	f(34, 16)
}
