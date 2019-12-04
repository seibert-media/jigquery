package function

import (
	"context"
	"flag"
	"time"

	"github.com/seibert-media/golibs/log"
	"go.uber.org/zap"
)

var ignoreLastRun = flag.Bool("ignoreLastRun", false, "ignores the last execution time and processes all issues again")

// InsertIssues into bigquery
func InsertIssues(ctx context.Context, env Environment) error {
	fields, err := GetSchema(ctx, env.SchemaBucket, env.SchemaPath)
	if err != nil {
		log.From(ctx).Error("reading schema", zap.String("bucket", env.SchemaBucket), zap.String("path", env.SchemaPath), zap.Error(err))
		return err
	}

	log.From(ctx).Debug("creating bigquery client")
	bigquery, err := NewBigQueryClient(ctx, env)
	if err != nil {
		log.From(ctx).Error("creating bigquery client", zap.Error(err))
		return err
	}

	if err := bigquery.Prepare(ctx, fields); err != nil {
		return err
	}

	var exec Execution

	if !*ignoreLastRun {
		log.From(ctx).Debug("fetching last execution")
		exec, err = bigquery.LastExecution(ctx)
		if err != nil {
			log.From(ctx).Error("fetching last execution", zap.Error(err))
		}
	}

	var now = time.Now().UTC()

	log.From(ctx).Debug("creating jira client")
	jira, err := NewJiraClient(ctx, env)
	if err != nil {
		log.From(ctx).Error("creating jira client", zap.Error(err))
		return err
	}

	log.From(ctx).Info("fetching issues")
	issues, err := jira.Issues(ctx, exec.Timestamp)
	if err != nil {
		log.From(ctx).Error("fetching issues", zap.Error(err))
		return err
	}

	converter := FieldExtractor(fields)

	log.From(ctx).Debug("converting issues")
	converted, err := converter.ExtractFromIssues(ctx, issues)
	if err != nil {
		log.From(ctx).Error("converting issues", zap.Error(err))
		return err
	}

	log.From(ctx).Info("inserting")
	if err := bigquery.Insert(ctx, converted); err != nil {
		log.From(ctx).Error("inserting", zap.Error(err))
		return err
	}

	if err := bigquery.RecordExecution(ctx, now, len(issues)); err != nil {
		return err
	}

	log.From(ctx).Info("inserted", zap.Int("issues", len(converted)))
	return nil
}
