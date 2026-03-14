package audio

// NoiseSuppressionType represents the type of noise suppression algorithm
type NoiseSuppressionType string

const (
	// NoiseSuppressionTypeRNNoise uses the RNNoise library (CGO)
	NoiseSuppressionTypeRNNoise NoiseSuppressionType = "rnnoise"
	// NoiseSuppressionTypeONNX uses the DFN ONNX model
	NoiseSuppressionTypeONNX NoiseSuppressionType = "dfn"
)

// String returns the string representation of the noise suppression type
func (n NoiseSuppressionType) String() string {
	return string(n)
}
