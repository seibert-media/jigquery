package function

import (
	"strings"

	"cloud.google.com/go/bigquery"
)

// FieldSchema represents the config for a single field
type FieldSchema struct {
	Name     string `json:"name,omitempty"`
	Type     string `json:"type,omitempty"`
	Path     string `json:"path,omitempty"`
	Required bool   `json:"required,omitempty"`
	Repeated bool   `json:"repeated,omitempty"`
}

// BigQuerySchema from the provided schema
func BigQuerySchema(from []FieldSchema) bigquery.Schema {

	var fieldSchemas []*bigquery.FieldSchema
	for _, field := range from {
		fieldSchemas = append(fieldSchemas, &bigquery.FieldSchema{
			Name:     field.Name,
			Type:     bigquery.FieldType(strings.ToUpper(field.Type)),
			Repeated: field.Repeated,
			Required: field.Required,
		})
	}

	return bigquery.Schema(fieldSchemas)
}
