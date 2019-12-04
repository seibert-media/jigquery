package function

import "testing"

import "encoding/json"

func TestFieldExtractionHandlesEmptyRepeatedFields(t *testing.T) {

	fields := []FieldSchema{
		FieldSchema{
			Name:     "nonRepeated",
			Type:     "string",
			Path:     "fields.nonRepeated",
			Repeated: false,
		},
		FieldSchema{
			Name:     "repeated",
			Type:     "string",
			Path:     "fields.repeated",
			Repeated: true,
		},
	}

	from := map[string]interface{}{
		"fields": map[string]interface{}{
			"nonRepeated": "foo",
		},
	}

	extractor := FieldExtractor(fields)

	result := make(map[string]interface{})
	for _, field := range fields {
		if err := extractor.extractField(field, from, result); err != nil {
			t.Fatal("extracting", err)
		}
	}

	expect := `{"nonRepeated":"foo","repeated":[]}`

	got, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != expect {
		t.Fatalf("got invalid field: %v\nexpected: %v", string(got), expect)
	}
}
