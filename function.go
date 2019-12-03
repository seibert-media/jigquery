package jigquery

import (
	"context"
	"os"

	"github.com/seibert-media/jigquery/function"

	"github.com/seibert-media/golibs/log"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	// Env for the current function instance
	env function.Environment
	// Logger for the current function instance
	logger *log.Logger
)

func init() {
	var err error

	env = function.ParseEnvironment()

	logger, err = log.New("", len(os.Getenv("LOCAL")) > 0)
	if err != nil {
		panic(err)
	}

	if len(os.Getenv("DEBUG")) > 0 {
		logger.SetLevel(zapcore.DebugLevel)
	}
}

// PubSubMessage is the payload of a Pub/Sub event.
type PubSubMessage struct {
	Data []byte `json:"data"`
}

// InsertIssues into BigQuery
func InsertIssues(ctx context.Context, m PubSubMessage) error {
	ctx = log.WithLogger(ctx, logger)

	log.From(ctx).Debug("validating environment")
	if err := env.Validate(); err != nil {
		log.From(ctx).Error("validating environment", zap.Error(err))
		return err
	}

	if err := function.InsertIssues(ctx, env); err != nil {
		return err
	}

	return nil
}
