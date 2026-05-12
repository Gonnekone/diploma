package model

import (
	"fmt"
	"math"
	"math/cmplx"

	ort "github.com/yalue/onnxruntime_go"
)

type Suppressor struct {
	session *ort.DynamicAdvancedSession
}

func NewSuppressor(modelPath string) (*Suppressor, error) {
	if err := ort.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("onnxruntime init: %w", err)
	}

	session, err := ort.NewDynamicAdvancedSession(
		modelPath,
		[]string{"input"},
		[]string{"output"},
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("create onnx session: %w", err)
	}

	return &Suppressor{session: session}, nil
}

func (s *Suppressor) Denoise(samples []float32) ([]float32, error) {
	frames := STFT(samples)
	if len(frames) == 0 {
		return samples, nil
	}
	T := len(frames)

	inputData := make([]float32, FreqBins*T)
	for f := 0; f < FreqBins; f++ {
		for t := 0; t < T; t++ {
			m := float32(cmplx.Abs(frames[t][f]))
			inputData[f*T+t] = float32(math.Log(float64(m) + 1e-6))
		}
	}

	inputShape := ort.NewShape(1, 1, int64(FreqBins), int64(T))
	inputTensor, err := ort.NewTensor(inputShape, inputData)
	if err != nil {
		return nil, fmt.Errorf("create input tensor: %w", err)
	}
	defer inputTensor.Destroy()

	outputShape := ort.NewShape(1, int64(FreqBins), int64(T))
	outputData := make([]float32, FreqBins*T)
	outputTensor, err := ort.NewTensor(outputShape, outputData)
	if err != nil {
		return nil, fmt.Errorf("create output tensor: %w", err)
	}
	defer outputTensor.Destroy()

	if err = s.session.Run(
		[]ort.ArbitraryTensor{inputTensor},
		[]ort.ArbitraryTensor{outputTensor},
	); err != nil {
		return nil, fmt.Errorf("model run: %w", err)
	}

	mask := outputTensor.GetData()

	predFrames := make([][]complex128, T)
	for t := 0; t < T; t++ {
		predFrames[t] = make([]complex128, FreqBins)
		for f := 0; f < FreqBins; f++ {
			m := float64(mask[f*T+t])
			mag := cmplx.Abs(frames[t][f])
			phase := cmplx.Phase(frames[t][f])
			predFrames[t][f] = cmplx.Rect(mag*m, phase)
		}
	}

	return ISTFT(predFrames, len(samples)), nil
}

func (s *Suppressor) Close() {
	s.session.Destroy()
	ort.DestroyEnvironment()
}
