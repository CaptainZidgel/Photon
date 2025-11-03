package main

import (
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
		{name: "No resize necessary", oldW: 1000, oldH: 1000},
		{name: "Square downsize", oldW: 10000, oldH: 10000},
		{name: "Rectangular downsize, larger width", oldW: 5000, oldH: 4000},
		{name: "Rectangular downsize, larger height", oldW: 4000, oldH: 5000},
		{name: "Square upsize", oldW: 5, oldH: 5, newW: minWidth, newH: minWidth},
		{name: "Rectangular upsize, larger width", oldW: 5, oldH: 4},
		{name: "Rectangular upsize, larger height", oldW: 4, oldH: 5},
		{name: "real example", oldW: 1000, oldH: 600},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			gotW, gotH := DetermineNewSize(test.oldW, test.oldH)
			if (gotW > maxWidth || gotW < minWidth) || (gotH > maxHeight || gotH < minHeight) {
				t.Errorf("Determine new size miscalculation. Old vals: %vx%v. Got: %vx%v\n", test.oldW, test.oldH, gotW, gotH)
			}
		})
	}
}
