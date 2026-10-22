package service

import nanoid "github.com/matoous/go-nanoid/v2"

const UpperAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

func randomUpper(n int) string {
	v, err := nanoid.Generate(UpperAlphabet, n)
	if err != nil {
		if n <= 0 {
			return ""
		}
		out := make([]byte, n)
		for i := range out {
			out[i] = 'A'
		}
		return string(out)
	}
	return v
}
