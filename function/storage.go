package function

import (
	"context"
	"encoding/json"

	"cloud.google.com/go/storage"
	"github.com/seibert-media/golibs/log"
	"go.uber.org/zap"
)

// GetSchema from the projects bucket and path
func GetSchema(ctx context.Context, bucket, path string) ([]FieldSchema, error) {

	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	obj := client.Bucket(bucket).Object(path)

	reader, err := obj.NewReader(ctx)
	if err != nil {
		return nil, err
	}

	var fields []FieldSchema

	log.From(ctx).Debug("parsing schema")
	if err := json.NewDecoder(reader).Decode(&fields); err != nil {
		log.From(ctx).Fatal("parsing schema", zap.Error(err))
	}

	return fields, nil
} 
