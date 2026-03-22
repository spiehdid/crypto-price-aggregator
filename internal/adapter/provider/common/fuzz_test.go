package common_test

import (
	"bytes"
	"encoding/json"
	"testing"
)

func FuzzDecodeJSON(f *testing.F) {
	// Seed corpus
	f.Add([]byte(`{"bitcoin":{"usd":67432.15}}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"a":{"b":0}}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`[]`))
	f.Add([]byte(``))
	f.Add([]byte(`{invalid`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var target map[string]map[string]json.Number
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()
		// Should never panic — only return error or succeed
		_ = dec.Decode(&target)
	})
}
