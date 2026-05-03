package model

import (
	"fmt"
	"math"
	"math/cmplx"

	ort "github.com/yalue/onnxruntime_go"
)

// Suppressor — обёртка над ONNX-моделью шумоподавления
type Suppressor struct {
	session *ort.DynamicAdvancedSession
}

// NewSuppressor загружает ONNX-модель по указанному пути
func NewSuppressor(modelPath string) (*Suppressor, error) {
	if err := ort.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("onnxruntime init: %w", err)
	}

	session, err := ort.NewDynamicAdvancedSession(
		modelPath,
		[]string{"input"},  // имя входного узла из экспорта
		[]string{"output"}, // имя выходного узла из экспорта
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("create onnx session: %w", err)
	}

	return &Suppressor{session: session}, nil
}

// Denoise принимает PCM float32 и возвращает очищенный от шума PCM float32
func (s *Suppressor) Denoise(samples []float32) ([]float32, error) {
	frames := STFT(samples)
	if len(frames) == 0 {
		return samples, nil
	}
	T := len(frames)

	// строим лог-амплитудную спектрограмму [1, 1, FreqBins, T]
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

	// выходной тензор: маска [1, FreqBins, T]
	outputShape := ort.NewShape(1, int64(FreqBins), int64(T))
	outputData := make([]float32, FreqBins*T)
	outputTensor, err := ort.NewTensor(outputShape, outputData)
	if err != nil {
		return nil, fmt.Errorf("create output tensor: %w", err)
	}
	defer outputTensor.Destroy()

	// инференс модели
	if err = s.session.Run(
		[]ort.ArbitraryTensor{inputTensor},
		[]ort.ArbitraryTensor{outputTensor},
	); err != nil {
		return nil, fmt.Errorf("model run: %w", err)
	}

	mask := outputTensor.GetData()

	// применяем маску: предсказанная амплитуда × исходная фаза
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

// Close освобождает ресурсы ONNX Runtime
func (s *Suppressor) Close() {
	s.session.Destroy()
	ort.DestroyEnvironment()
}
