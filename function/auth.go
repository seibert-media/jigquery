package function

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

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

// CreateKeys in the project
func CreateKeys(ctx context.Context, project string, keyRing, key string) error {
	location := fmt.Sprintf("projects/%s/locations/global", project)

	client, err := kms.NewKeyManagementClient(ctx)
	if err != nil {
		return err
	}

	ringReq := &kmspb.CreateKeyRingRequest{
		Parent:    location,
		KeyRingId: keyRing,
	}

	if _, err := client.CreateKeyRing(ctx, ringReq); err != nil {
		return err
	}

	keyReq := &kmspb.CreateCryptoKeyRequest{
		Parent:      fmt.Sprintf("%s/keyRings/%s", location, keyRing),
		CryptoKeyId: key,
		CryptoKey: &kmspb.CryptoKey{
			Purpose: kmspb.CryptoKey_ENCRYPT_DECRYPT,
			VersionTemplate: &kmspb.CryptoKeyVersionTemplate{
				Algorithm: kmspb.CryptoKeyVersion_GOOGLE_SYMMETRIC_ENCRYPTION,
			},
		},
	}

	if _, err = client.CreateCryptoKey(ctx, keyReq); err != nil {
		return err
	}

	return nil
}
