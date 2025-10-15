package main

import (
	"bytes"
	"math"
)

// generateBlipPCM creates a short sine blip as 16-bit little-endian stereo PCM.
func generateBlipPCM(sampleRate int, seconds float64, freqHz float64) []byte {
	n := int(float64(sampleRate) * seconds)
	var b bytes.Buffer
	amp := 0.25 // 25% of full-scale to avoid clipping
	for i := 0; i < n; i++ {
		phase := 2 * math.Pi * float64(i) * freqHz / float64(sampleRate)
		// Apply simple exponential decay
		decay := math.Exp(-6 * float64(i) / float64(n))
		s := math.Sin(phase) * amp * decay
		v := int16(s * 32767)
		// stereo: L then R
		b.WriteByte(byte(v))
		b.WriteByte(byte(v >> 8))
		b.WriteByte(byte(v))
		b.WriteByte(byte(v >> 8))
	}
	return b.Bytes()
}
