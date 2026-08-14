// Package jsonv1 提供 Astrahold Reliable control/state message 的 strict JSON Payload Codec。
package jsonv1

import "errors"

var (
	ErrUnsupportedMessage = errors.New("jsonv1: unsupported message")
	ErrTrailingData       = errors.New("jsonv1: trailing data")
)

type Codec struct{}
