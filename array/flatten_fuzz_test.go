package array

import "testing"

func FuzzFlattenDeep(f *testing.F) {
	f.Add([]byte(`[[1,2],[3]]`))
	f.Add([]byte(`{"a":1}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1024 {
			return
		}

		defer func() {
			if recover() != nil {
				t.Fatalf("FlattenDeep panicked on input len=%d", len(data))
			}
		}()

		_ = FlattenDeep(data)
		_ = FlattenDeep(string(data))
		_ = FlattenDeep([]any{data, []any{1, []any{2}}})
	})
}
