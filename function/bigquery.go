package function

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/seibert-media/golibs/log"
	"go.uber.org/zap"
	"google.golang.org/api/googleapi"
)

// BigQueryClient wraps a bigquery.Client to provide helpers
type BigQueryClient struct {
	*bigquery.Client
	//Project   string
	Dataset   *bigquery.Dataset
	Table     *bigquery.Table
	ExecTable *bigquery.Table
}

// NewBigQueryClient for the provided environment
func NewBigQueryClient(ctx context.Context, env Environment) (*BigQueryClient, error) {
	client, err := bigquery.NewClient(ctx, env.BigQueryProject)
	if err != nil {
		return nil, err
	}

	dataset := client.Dataset(env.BigQueryDataset)

	return &BigQueryClient{
		Client: client,
		//Project:   env.GoogleProject,
		Dataset:   dataset,
		Table:     dataset.Table(env.BigQueryTable),
		ExecTable: dataset.Table(fmt.Sprintf("%s_executions", env.BigQueryTable)),
	}, nil
}

// Prepare the client by creating it's dataset and table
func (c *BigQueryClient) Prepare(ctx context.Context, fields []FieldSchema) error {
	log.From(ctx).Debug("creating dataset")
	if err := c.CreateDataset(ctx); err != nil {
		log.From(ctx).Error("creating dataset", zap.Error(err))
		return err
	}

	log.From(ctx).Debug("creating table")
	if err := c.CreateTable(ctx, BigQuerySchema(fields)); err != nil {
		log.From(ctx).Error("creating table", zap.Error(err))
		return err
	}

	return nil
}

// CreateDataset if it does not exist
func (c *BigQueryClient) CreateDataset(ctx context.Context) error {
	if err := c.Dataset.Create(ctx, &bigquery.DatasetMetadata{}); err != nil && !isExists(err) {
		log.From(ctx).Error("creating dataset", zap.Error(err))
		return err
	}
	return nil
}

// CreateTable and the respective executions table, if it does not exist
func (c *BigQueryClient) CreateTable(ctx context.Context, schema bigquery.Schema) error {
	if err := c.Table.Create(ctx, &bigquery.TableMetadata{Schema: schema}); err != nil && !isExists(err) {
		log.From(ctx).Error("creating table", zap.Error(err))
		return err
	}

	if err := c.ExecTable.Create(ctx, &bigquery.TableMetadata{Schema: bigquery.Schema{
		&bigquery.FieldSchema{Name: "timestamp", Required: true, Type: bigquery.TimestampFieldType},
		&bigquery.FieldSchema{Name: "inserted", Required: true, Type: bigquery.IntegerFieldType},
	}, TimePartitioning: &bigquery.TimePartitioning{Field: "timestamp"}}); err != nil && !isExists(err) {
		log.From(ctx).Error("creating executions table", zap.Error(err))
		return err
	}

	return nil
}

// Insert into the client's table
func (c *BigQueryClient) Insert(ctx context.Context, issues []Issue) error {
	inserter := c.Table.Inserter()
	inserter.IgnoreUnknownValues = true

	if err := inserter.Put(ctx, issues); err != nil {
		putErr := err.(bigquery.PutMultiError)
		for _, rowErr := range putErr {
			log.From(ctx).Error("inserting row", zap.Error(rowErr.Errors))
		}

		return err
	}

	return nil
}

// Execution of the inserter
type Execution struct {
	Timestamp time.Time `json:"timestamp,omitempty"`
	Inserted  int       `json:"inserted,omitempty"`
}

// RecordExecution in the client's execution table
func (c *BigQueryClient) RecordExecution(ctx context.Context, at time.Time, inserted int) error {
	inserter := c.ExecTable.Inserter()
	inserter.IgnoreUnknownValues = true

	if err := inserter.Put(ctx, Execution{Timestamp: at, Inserted: inserted}); err != nil {
		putErr := err.(bigquery.PutMultiError)
		for _, rowErr := range putErr {
			log.From(ctx).Error("inserting row", zap.Error(rowErr.Errors))
		}

		return err
	}
	return nil
}

// LastExecution .
func (c *BigQueryClient) LastExecution(ctx context.Context) (Execution, error) {
	rows, err := c.Query(fmt.Sprintf("SELECT * FROM `%s.%s.%s` ORDER BY timestamp DESC LIMIT 1", c.ExecTable.ProjectID, c.ExecTable.DatasetID, c.ExecTable.TableID)).Read(ctx)
	if err != nil {
		return Execution{}, err
	}

	var row Execution
	if err := rows.Next(&row); err != nil {
		return Execution{}, err
	}

	return row, nil
}

func isExists(err error) bool {
	if gerr, ok := err.(*googleapi.Error); ok {
		if gerr.Code == http.StatusConflict {
			return true
		}
	}
	return false
}

func isNotFound(err error) bool {
	if gerr, ok := err.(*googleapi.Error); ok {
		if gerr.Code == http.StatusNotFound {
			return true
		}
	}
	return false
}
