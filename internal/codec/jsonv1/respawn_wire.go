package jsonv1

// clientRespawnRequest intentionally has no fields. JSON v1 encodes it as `{}` while the
// production gamev1 codec uses a zero-byte payload for the same protocol intent.
type clientRespawnRequest struct{}
