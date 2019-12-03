package function

import (
	"fmt"
	"os"
)

// Environment variables for running the function
type Environment struct {
	JiraAuthResource string
	JiraAuthSecret   string
	JiraProject      string

	SchemaBucket string
	SchemaPath   string

	BigQueryProject string
	BigQueryDataset string
	BigQueryTable   string
}

// ParseEnvironment variables into an Environment
func ParseEnvironment() Environment {
	return Environment{
		JiraAuthResource: os.Getenv("JIRA_AUTH_RESOURCE"),
		JiraAuthSecret:   os.Getenv("JIRA_AUTH_SECRET"),
		JiraProject:      os.Getenv("JIRA_PROJECT"),
		// JiraQuery:    os.Getenv("JIRA_QUERY"),

		SchemaBucket: os.Getenv("SCHEMA_BUCKET"),
		SchemaPath:   os.Getenv("SCHEMA_PATH"),

		BigQueryProject: os.Getenv("BIGQUERY_PROJECT"),
		BigQueryDataset: os.Getenv("BIGQUERY_DATASET"),
		BigQueryTable:   os.Getenv("BIGQUERY_TABLE"),
	}
}

// Validate the environment
func (e Environment) Validate() error {
	if len(e.JiraAuthResource) < 1 {
		return fmt.Errorf("missing environment variable: %s", "JIRA_AUTH_RESOURCE")
	}
	if len(e.JiraAuthSecret) < 1 {
		return fmt.Errorf("missing environment variable: %s", "JIRA_AUTH_SECRET")
	}
	if len(e.JiraProject) < 1 {
		return fmt.Errorf("missing environment variable: %s", "JIRA_PROJECT")
	}
	if len(e.SchemaPath) < 1 {
		return fmt.Errorf("missing environment variable: %s", "SCHEMA_PATH")
	}
	if len(e.SchemaBucket) < 1 {
		return fmt.Errorf("missing environment variable: %s", "SCHEMA_BUCKET")
	}
	if len(e.BigQueryProject) < 1 {
		return fmt.Errorf("missing environment variable: %s", "BIGQUERY_PROJECT")
	}
	if len(e.BigQueryDataset) < 1 {
		return fmt.Errorf("missing environment variable: %s", "BIGQUERY_DATASET")
	}
	if len(e.BigQueryTable) < 1 {
		return fmt.Errorf("missing environment variable: %s", "BIGQUERY_TABLE")
	}

	return nil
}
