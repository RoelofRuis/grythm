package main

import (
	"bytes"
	"math"
)

// generateBlipPCM creates a short, pleasant blip as 16-bit little-endian stereo PCM.
// It applies a short fade-in (attack), gentle exponential decay, a subtle downward
// pitch glide, and a very quiet second harmonic for a warmer tone. Stereo channels
// are given a tiny phase/pan difference for width.
func generateBlipPCM(sampleRate int, seconds float64, freqHz float64) []byte {
	n := int(float64(sampleRate) * seconds)
	if n <= 0 {
		return nil
	}
	var b bytes.Buffer

	// Overall amplitude kept conservative to avoid clipping when harmonics combine.
	amp := 0.22

	// Envelope: short attack to avoid clicks, then exponential decay.
	attackSec := math.Min(0.005, seconds*0.2) // up to 5ms
	attackN := int(attackSec * float64(sampleRate))
	// Decay so that we reach about -60dB near the end of the sample.
	// exp(-lambda) ~ 0.001 -> lambda ~ 6.9; distribute over n samples.
	lambda := 6.9

	// Pitch glide: slight downwards bend for a softer percussive feel.
	startFreq := freqHz * 1.03
	endFreq := freqHz * 0.92

	// Stereo: tiny phase offset and pan difference.
	phaseOffsetR := 0.015 // radians
	panL := 0.55
	panR := 0.45

	phase := 0.0
	for i := 0; i < n; i++ {
		// Time fraction 0..1
		t := float64(i) / float64(n-1)

		// Smooth attack
		var envA float64
		if i < attackN && attackN > 0 {
			// cosine fade-in: 0 -> 1 smoothly
			x := float64(i) / float64(attackN)
			envA = 0.5 - 0.5*math.Cos(math.Pi*x)
		} else {
			envA = 1.0
		}
		// Exponential decay over the note duration
		envD := math.Exp(-lambda * t)
		env := amp * envA * envD

		// Exponential-ish glide by interpolating frequency in log domain
		f := startFreq * math.Pow(endFreq/startFreq, t)
		phase += 2 * math.Pi * f / float64(sampleRate)

		// Base sine plus a very quiet second harmonic
		base := math.Sin(phase)
		second := math.Sin(2 * phase) * 0.18
		mono := (base + second) * env

		// Stereo with tiny right-channel phase offset and pan
		l := mono * panL
		r := math.Sin(phase+phaseOffsetR)*env*panR + second*env*0.18*panR

		// Convert to 16-bit
		lv := int16(max(-1, min(1, l)) * 32767)
		rv := int16(max(-1, min(1, r)) * 32767)

		// Write little-endian stereo L then R
		b.WriteByte(byte(lv))
		b.WriteByte(byte(lv >> 8))
		b.WriteByte(byte(rv))
		b.WriteByte(byte(rv >> 8))
	}
	return b.Bytes()
}

func min(a, b float64) float64 { if a < b { return a }; return b }
func max(a, b float64) float64 { if a > b { return a }; return b }
