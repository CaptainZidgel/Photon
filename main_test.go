package main

import (
	"math"
	"testing"
)

func TestDetermineNewSize(t *testing.T) {
	var tests = []struct {
		name string
		oldW int
		oldH int
		newW int
		newH int
	}{
		{name: "No resize necessary", oldW: 1000, oldH: 1000, newW: 1000, newH: 1000},
		{name: "Square downsize", oldW: 10000, oldH: 10000, newW: maxWidth, newH: maxHeight},
		{name: "Rectangular downsize, larger width", oldW: 5000, oldH: 4000, newW: maxWidth, newH: int(math.Round(822.4))},
		{name: "Rectangular downsize, larger height", oldW: 4000, oldH: 5000, newW: int(math.Round(822.4)), newH: maxHeight},
		{name: "Square upsize", oldW: 100, oldH: 100, newW: minWidth, newH: minWidth},
		{name: "Rectangular upsize, larger width", oldW: 50, oldH: 40, newW: 320, newH: minHeight},
		{name: "Rectangular upsize, larger height", oldW: 40, oldH: 50, newW: minWidth, newH: 320},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			gotW, gotH := DetermineNewSize(test.oldW, test.oldH)
			if gotW != test.newW || gotH != test.newH {
				t.Errorf("Determine new size miscalculation. Old vals: %vx%v. Wanted: %vx%v. Got: %vx%v\n", test.oldW, test.oldH, test.newW, test.newH, gotW, gotH)
			}
		})
	}
}
