package function

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	jira "github.com/andygrunwald/go-jira"
	"github.com/seibert-media/golibs/log"
	"go.uber.org/zap"
)

// Issue to be stored
type Issue map[string]interface{}

// Save implements bigquery.ValueSaver
// It takes care of transforming a Jira Timestamp into a BigQuery Timestamp
func (i Issue) Save() (map[string]bigquery.Value, string, error) {
	values := make(map[string]bigquery.Value)
	for key, value := range i {
		if time, err := time.Parse("2006-01-02T15:04:05.999-0700", fmt.Sprint(value)); err == nil {
			value = time.UTC().Format("2006-01-02 15:04:05.999999")
		}
		values[key] = value
	}

	return values, "", nil
}

// JiraClient wraps a jira.Client to provided helpers
type JiraClient struct {
	*jira.Client
	Project string
}

// NewJiraClient from the passed in environment
func NewJiraClient(ctx context.Context, env Environment) (JiraClient, error) {
	auth, err := DecodeJiraAuth(ctx, env.JiraAuthResource, env.JiraAuthSecret)
	if err != nil {
		return JiraClient{}, err
	}

	tp := jira.BasicAuthTransport{
		Username: auth.Username,
		Password: auth.Password,
	}

	jira, err := jira.NewClient(tp.Client(), auth.URL)
	if err != nil {
		return JiraClient{}, err
	}

	return JiraClient{Client: jira, Project: env.JiraProject}, nil
}

// Issues from the client's jira instance
func (c JiraClient) Issues(ctx context.Context, lastRun time.Time) ([]Issue, error) {

	filter := ""
	if !lastRun.IsZero() {
		lastRun, err := c.inUserTimezone(lastRun)
		if err != nil {
			return nil, err
		}

		// give 2 minute of buffer so we make sure to not miss any issues
		lastRun = lastRun.Add(-2 * time.Minute)

		filter = fmt.Sprintf(` AND updated >= "%s"`, lastRun.Format("2006-01-02 15:04"))
	}

	jql := fmt.Sprintf("project = %s%s ORDER BY updated ASC", c.Project, filter)

	issues, err := c.Search(ctx, jql, &jira.SearchOptions{MaxResults: 500})

	return issues, err
}

// Search without decoding to the jira.Issue type, to get all fields without modification
// If not all issues can be acquired in a single call to jira, pagination will be used to get the full set of issues
// This can take some while depending on the speed of jira and the amount of issues and should
// be respected when defining timeouts (e.g. function runtime)
func (c JiraClient) Search(ctx context.Context, jql string, options *jira.SearchOptions) ([]Issue, error) {
	issues := []Issue{}
	total := -1

	for len(issues) < total || total == -1 {
		log.From(ctx).Debug("reading page", zap.Int("current", len(issues)), zap.Int("total", total), zap.Int("startAt", options.StartAt), zap.Int("maxResults", options.MaxResults))

		resp, err := c.search(urlFromOptions(jql, options))
		if err != nil {
			return nil, err
		}

		issues = append(issues, resp.Issues...)
		total = resp.Total

		options.StartAt = resp.StartAt + resp.MaxResults
		options.MaxResults = resp.MaxResults
	}

	return issues, nil
}

type searchResponse struct {
	StartAt    int     `json:"startAt"`
	MaxResults int     `json:"maxResults"`
	Total      int     `json:"total"`
	Issues     []Issue `json:"issues"`
}

func (c JiraClient) search(url string) (searchResponse, error) {
	req, err := c.NewRequest("GET", url, nil)
	if err != nil {
		return searchResponse{}, err
	}

	var body searchResponse
	resp, err := c.Do(req, &body)
	if err != nil {
		return searchResponse{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return searchResponse{}, fmt.Errorf("searching issues: status %v", resp.StatusCode)
	}

	return body, nil
}

func urlFromOptions(jql string, options *jira.SearchOptions) string {
	reqURL := fmt.Sprintf("rest/api/2/search?jql=%s", url.QueryEscape(jql))

	if options != nil {
		if options.MaxResults != 0 {
			reqURL += fmt.Sprintf("&maxResults=%d", options.MaxResults)
		}
		if options.StartAt != 0 {
			reqURL += fmt.Sprintf("&startAt=%d", options.StartAt)
		}
	}

	return reqURL
}

// inUserTimezone converts the provided time.Time to the timezone for the current Jira user
func (c JiraClient) inUserTimezone(t time.Time) (time.Time, error) {
	self, _, err := c.Client.User.GetSelf()
	if err != nil {
		return t, err
	}
	timezone := self.TimeZone
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return t, err
	}
	return t.In(loc), nil
}

// FieldExtractor for a certain schema
type FieldExtractor []FieldSchema

// ExtractFromIssues extracts the fields defined in the extractor from the provided issues
func (extractor FieldExtractor) ExtractFromIssues(ctx context.Context, issues []Issue) ([]Issue, error) {
	var internal []Issue
	for _, issue := range issues {
		i, err := extractor.extractFromIssue(ctx, issue)
		if err != nil {
			return nil, err
		}
		internal = append(internal, i)
	}

	return internal, nil
}

func (extractor FieldExtractor) extractFromIssue(ctx context.Context, issue Issue) (Issue, error) {
	issueKey, hasKey := issue["key"]
	if !hasKey {
		return nil, fmt.Errorf("invalid issue: missing key: %v", issue)
	}
	log.From(ctx).Debug("handling issue", zap.String("key", issueKey.(string)))

	result := make(map[string]interface{})
	for _, field := range extractor {
		if err := extractor.extractField(field, issue, result); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// extractField from the provided fields by traversing the from object based on the field.Path and add it into the map based on it's field.Name
func (extractor FieldExtractor) extractField(field FieldSchema, from, into map[string]interface{}) error {
	fieldPath := buildFieldPath(field.Path)

	var level map[string]interface{} = from
	for i, step := range fieldPath {

		cur, ok := level[step]
		if !ok && field.Required {
			return newPathError(fieldPath[i], fieldPath[:i])
		}

		if level, ok = cur.(map[string]interface{}); !ok {
			if i < len(fieldPath)-1 && field.Required {
				return newPathError(fieldPath[i], fieldPath[:i])
			}

			// set empty repeated fields to an empty list as bigquery does not like nulled repeated fields
			// ref: https://github.com/googleapis/google-cloud-python/issues/9602
			if cur == nil && field.Repeated {
				into[field.Name] = []interface{}{}
			} else {
				into[field.Name] = cur
			}
		}
	}

	return nil
}

func newPathError(path string, fullPath []string) error {
	return fmt.Errorf(
		"path not found %v at %v",
		path,
		strings.Join(fullPath, "."),
	)
}

// buildFieldPath splits a string into separate path steps
func buildFieldPath(from string) []string {
	return strings.Split(from, ".")
}
