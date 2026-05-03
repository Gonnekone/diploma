package model

import (
	"math"
	"math/cmplx"

	"github.com/madelynnblue/go-dsp/fft"
)

const (
	SampleRate  = 48000
	FrameLength = 512
	HopLength   = 128
	FreqBins    = FrameLength/2 + 1 // 257
)

// hannWindow возвращает окно Хэнна длиной n
func hannWindow(n int) []float64 {
	w := make([]float64, n)
	for i := range w {
		w[i] = 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(n-1)))
	}
	return w
}

// STFT — кратковременное преобразование Фурье
// Принимает PCM float32, возвращает [T][FreqBins]complex128
func STFT(samples []float32) [][]complex128 {
	window := hannWindow(FrameLength)
	numFrames := (len(samples)-FrameLength)/HopLength + 1
	if numFrames <= 0 {
		return nil
	}

	frames := make([][]complex128, numFrames)
	for t := 0; t < numFrames; t++ {
		start := t * HopLength
		frame := make([]complex128, FrameLength)
		for i := 0; i < FrameLength; i++ {
			if start+i < len(samples) {
				frame[i] = complex(float64(samples[start+i])*window[i], 0)
			}
		}
		spectrum := fft.FFT(frame)
		// берём только первую половину (FreqBins = N/2 + 1)
		frames[t] = make([]complex128, FreqBins)
		copy(frames[t], spectrum[:FreqBins])
	}
	return frames
}

// ISTFT — обратное STFT с overlap-add восстановлением
// Принимает [T][FreqBins]complex128, возвращает PCM float32
func ISTFT(frames [][]complex128, length int) []float32 {
	window := hannWindow(FrameLength)
	output := make([]float64, length)
	windowSum := make([]float64, length)

	for t, frame := range frames {
		// восстанавливаем полный симметричный спектр для вещественного IFFT
		full := make([]complex128, FrameLength)
		copy(full, frame)
		for i := 1; i < FrameLength/2; i++ {
			full[FrameLength-i] = cmplx.Conj(frame[i])
		}

		timeFrame := fft.IFFT(full)

		start := t * HopLength
		for i := 0; i < FrameLength && start+i < length; i++ {
			output[start+i] += real(timeFrame[i]) * window[i]
			windowSum[start+i] += window[i] * window[i]
		}
	}

	// нормализация через сумму окон (overlap-add)
	result := make([]float32, length)
	for i := range result {
		if windowSum[i] > 1e-8 {
			result[i] = float32(output[i] / windowSum[i])
		}
	}
	return result
}
