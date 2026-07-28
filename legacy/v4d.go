package main

import (
	"math"
	"strconv"
	"strings"
)

type V4d struct {
	X float64
	Y float64
	Z float64
	A float64
}

// return the parsed V4d and the number of fields parsed, or an error
func ParseV4d(coords string) (V4d, int, error) {
	parts := strings.Split(coords, ",")

	var v V4d

	// assign to fields of v in order from X to A (for example we might have
	// fewer than 4 fields)
	coord := []*float64{&v.X, &v.Y, &v.Z, &v.A}
	for i, part := range parts {
		if i >= len(coord) {
			return v, len(coord), nil
		}

		var err error
		*coord[i], err = strconv.ParseFloat(part, 64)
		if err != nil {
			return V4d{}, 0, err
		}
	}

	return v, len(parts), nil
}

func (a V4d) Add(b V4d) V4d {
	return V4d{X: a.X + b.X, Y: a.Y + b.Y, Z: a.Z + b.Z, A: a.A + b.A}
}

func (a V4d) Sub(b V4d) V4d {
	return V4d{X: a.X - b.X, Y: a.Y - b.Y, Z: a.Z - b.Z, A: a.A - b.A}
}

func (a V4d) Mul(k float64) V4d {
	return V4d{X: a.X * k, Y: a.Y * k, Z: a.Z * k, A: a.Z * k}
}

func (a V4d) Div(k float64) V4d {
	return a.Mul(1 / k)
}

func (a V4d) Length() float64 {
	return math.Sqrt(a.X*a.X + a.Y*a.Y + a.Z*a.Z + a.A*a.A)
}
