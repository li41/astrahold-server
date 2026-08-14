package jsonv1

import (
	"bytes"
	"encoding/json"
	"io"
)

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil { return err }
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil { return ErrTrailingData }
		return err
	}
	return nil
}
