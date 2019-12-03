package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/seibert-media/jigquery/function"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/seibert-media/golibs/log"
	"go.uber.org/zap"
)

var (
	keyRing = flag.String("keyRing", "", "the google kms keyring to use for encrypting secrets")
	key     = flag.String("key", "", "the google kms key to use for encrypting secrets")
)

// JiraAuth stores an encrypted representation of a jigquery.JiraAuth object
type JiraAuth struct {
	Resource string `json:"resource,omitempty"`
	Secret   string `json:"secret,omitempty"`
}

// GenerateSecret from the provided path
func GenerateSecret(ctx context.Context, project string) (JiraAuth, error) {

	if len(*keyRing) < 1 {
		*keyRing = "jigquery"
	}
	if len(*key) < 1 {
		*key = "jigquery"
	}

	resource := fmt.Sprintf("projects/%s/locations/global/keyRings/%s/cryptoKeys/%s", project, *keyRing, *key)

	scanner := bufio.NewScanner(os.Stdin)

	jiraAuth := &function.JiraAuth{}

	fmt.Printf("Jira URL: ")
	scanner.Scan()
	if err := scanner.Err(); err != nil {
		log.From(ctx).Error("reading input", zap.Error(err))
		return JiraAuth{}, err
	}
	jiraAuth.URL = scanner.Text()

	fmt.Printf("Jira Username: ")
	scanner.Scan()
	if err := scanner.Err(); err != nil {
		log.From(ctx).Error("reading input", zap.Error(err))
		return JiraAuth{}, err
	}
	jiraAuth.Username = scanner.Text()

	fmt.Printf("Jira Password: ")
	scanner.Scan()
	if err := scanner.Err(); err != nil {
		log.From(ctx).Error("reading input", zap.Error(err))
		return JiraAuth{}, err
	}
	jiraAuth.Password = scanner.Text()

	secret, err := function.EncodeJiraAuth(ctx, resource, jiraAuth)
	if err != nil && !isNotFound(err) {
		log.From(ctx).Error("encoding jira auth", zap.Error(err))
		return JiraAuth{}, err
	}

	if isNotFound(err) {
		if err := function.CreateKeys(ctx, project, *keyRing, *key); err != nil {
			return JiraAuth{}, err
		}
		secret, err = function.EncodeJiraAuth(ctx, resource, jiraAuth)
		if err != nil {
			return JiraAuth{}, err
		}
	}

	auth := JiraAuth{
		Resource: resource,
		Secret:   secret,
	}

	file, err := os.OpenFile("./.auth.json", os.O_CREATE|os.O_RDWR, os.ModePerm)
	defer file.Close()

	if err := json.NewEncoder(file).Encode(auth); err != nil {
		log.From(ctx).Error("writing file", zap.String("file", "./.auth.json"), zap.Error(err))
		return JiraAuth{}, err
	}

	return auth, nil
}

func isNotFound(err error) bool {
	if gerr, ok := err.(*googleapi.Error); ok {
		if gerr.Code == http.StatusNotFound {
			return true
		}
	}

	if st, ok := status.FromError(err); ok {
		if st.Code() == codes.NotFound {
			return true
		}
	}

	return false
}
