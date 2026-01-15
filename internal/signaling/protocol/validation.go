package protocol

import "regexp"

// uuidV4Re matches UUIDv4 format.
var uuidV4Re = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-4[0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

// isValidUUID checks if the string is a valid UUIDv4.
func isValidUUID(s string) bool {
	return uuidV4Re.MatchString(s)
}

// isValidRequestMethod checks if the method is a known request method.
func isValidRequestMethod(method string) bool {
	switch method {
	case MethodJoin,
		MethodLeave,
		MethodPublish,
		MethodUnpublish,
		MethodSubscribe,
		MethodUnsubscribe,
		MethodSetPreferredLayer,
		MethodOffer,
		MethodAnswer,
		MethodCandidate:
		return true
	default:
		return false
	}
}

// isValidSimulcastLayer checks if the layer is a valid simulcast layer.
func isValidSimulcastLayer(layer SimulcastLayer) bool {
	switch layer {
	case SimulcastLayerHigh, SimulcastLayerMedium, SimulcastLayerLow:
		return true
	default:
		return false
	}
}
