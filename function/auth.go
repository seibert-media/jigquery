package function

import (
	"context"
	"encoding/base64"
	"encoding/json"

	kms "cloud.google.com/go/kms/apiv1"
	"github.com/seibert-media/golibs/log"
	"go.uber.org/zap"
	kmspb "google.golang.org/genproto/googleapis/cloud/kms/v1"
)

// JiraAuth contains the required Jira login information
type JiraAuth struct {
	URL      string `json:"url,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// DecodeJiraAuth from the provided resource and secret
func DecodeJiraAuth(ctx context.Context, resource, secret string) (*JiraAuth, error) {

	raw, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		return nil, err
	}

	client, err := kms.NewKeyManagementClient(ctx)
	if err != nil {
		return nil, err
	}

	req := &kmspb.DecryptRequest{
		Name:       resource,
		Ciphertext: raw,
	}

	// Call the API.
	resp, err := client.Decrypt(ctx, req)
	if err != nil {
		return nil, err
	}

	var auth *JiraAuth
	if err := json.Unmarshal(resp.GetPlaintext(), &auth); err != nil {
		return nil, err
	}

	return auth, nil
}

// EncodeJiraAuth for the provided resource
func EncodeJiraAuth(ctx context.Context, resource string, auth *JiraAuth) (string, error) {
	authJSON, err := json.Marshal(auth)
	if err != nil {
		log.From(ctx).Fatal("encoding auth", zap.Error(err))
	}

	client, err := kms.NewKeyManagementClient(ctx)
	if err != nil {
		return "", err
	}

	req := &kmspb.EncryptRequest{
		Name:      resource,
		Plaintext: authJSON,
	}

	// Call the API.
	resp, err := client.Encrypt(ctx, req)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(resp.GetCiphertext()), nil
}
