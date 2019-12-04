package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	scheduler "cloud.google.com/go/scheduler/apiv1"
	"cloud.google.com/go/storage"
	"github.com/pkg/errors"
	"github.com/seibert-media/golibs/log"
	"go.uber.org/zap"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/cloudfunctions/v1"
	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iam/v1"
	schedulerpb "google.golang.org/genproto/googleapis/cloud/scheduler/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	jiraProject     = flag.String("jiraProject", "", "the jira project to use")
	schemaFile      = flag.String("schemaFile", "./.schema.json", "the json file containing the schema")
	bigQueryDataset = flag.String("bigqueryDataset", "", "the dataset to use")
	bigQueryTable   = flag.String("bigqueryTable", "", "the table to store issues in")
)

// Deploy the function
func Deploy(ctx context.Context, project string) error {

	if *interactive {
		scanner := bufio.NewScanner(os.Stdin)

		fmt.Printf("Jira Project: ")
		scanner.Scan()
		if err := scanner.Err(); err != nil {
			log.From(ctx).Error("reading input", zap.Error(err))
			return err
		}
		*jiraProject = scanner.Text()

		fmt.Printf("BigQuery Dataset: ")
		scanner.Scan()
		if err := scanner.Err(); err != nil {
			log.From(ctx).Error("reading input", zap.Error(err))
			return err
		}
		*bigQueryDataset = scanner.Text()

		fmt.Printf("BigQuery Table: ")
		scanner.Scan()
		if err := scanner.Err(); err != nil {
			log.From(ctx).Error("reading input", zap.Error(err))
			return err
		}
		*bigQueryTable = scanner.Text()
	}

	if err := validateFlags(); err != nil {
		return err
	}

	var auth JiraAuth
	authFile, err := os.OpenFile("./.auth.json", os.O_RDONLY, os.ModePerm)
	if err == nil {
		defer authFile.Close()

		log.From(ctx).Debug("reading auth file", zap.String("path", "./.auth.json"))
		if err := json.NewDecoder(authFile).Decode(&auth); err != nil {
			log.From(ctx).Error("reading auth file", zap.Error(err), zap.String("path", "./.auth.json"))
			return err
		}
	} else {
		log.From(ctx).Warn("reading auth file", zap.Error(err))
	}

	if len(auth.Resource) < 1 || len(auth.Secret) < 1 {
		log.From(ctx).Warn("missing auth file")
		log.From(ctx).Info("creating auth file")
		auth, err = GenerateSecret(ctx, project)
		if err != nil {
			return err
		}
	}

	if err := uploadSchema(ctx); err != nil {
		return err
	}

	svc, err := cloudfunctions.NewService(ctx)
	if err != nil {
		return err
	}

	fnc := cloudfunctions.NewProjectsLocationsFunctionsService(svc)
	location := fmt.Sprintf("projects/%s/locations/europe-west2", project)

	log.From(ctx).Debug("generating upload url")
	uploadURL, err := fnc.GenerateUploadUrl(location, &cloudfunctions.GenerateUploadUrlRequest{}).Context(ctx).Do()
	if err != nil {
		return errors.Wrap(err, "generating upload url")
	}

	if uploadURL.HTTPStatusCode != http.StatusOK {
		log.From(ctx).Error("generating upload url", zap.Int("status", uploadURL.HTTPStatusCode))
		return fmt.Errorf("generating upload url: status: %v", uploadURL.HTTPStatusCode)
	}

	if err := uploadCode(ctx, uploadURL.UploadUrl); err != nil {
		return err
	}

	functionName := fmt.Sprintf("%s--%s_%s", *jiraProject, *bigQueryDataset, *bigQueryTable)
	topic := fmt.Sprintf("projects/%s/topics/%s", project, functionName)

	log.From(ctx).Debug("creating scheduler client")
	sched, err := scheduler.NewCloudSchedulerClient(ctx)
	if err != nil {
		log.From(ctx).Error("creating scheduler client", zap.Error(err))
		return err
	}

	log.From(ctx).Debug("creating scheduler job")
	if _, err := sched.CreateJob(ctx, &schedulerpb.CreateJobRequest{
		Parent: location,
		Job: &schedulerpb.Job{
			Name:     fmt.Sprintf("%s/jobs/%s", location, functionName),
			Schedule: "0 0 * * *",
			Target: &schedulerpb.Job_PubsubTarget{
				PubsubTarget: &schedulerpb.PubsubTarget{
					TopicName: topic,
					Data:      []byte("{}"),
				},
			},
		},
	}); err != nil && !isExists(err) {
		log.From(ctx).Error("creating scheduler job", zap.Error(err))
		return err
	}

	log.From(ctx).Debug("creating service account")
	serviceAccount, err := CreateServiceAccount(ctx, project)
	if err != nil {
		return err
	}

	function := &cloudfunctions.CloudFunction{
		Name:    fmt.Sprintf("%s/functions/%s", location, functionName),
		Runtime: "go111",
		EventTrigger: &cloudfunctions.EventTrigger{
			EventType: "providers/cloud.pubsub/eventTypes/topic.publish",
			Resource:  topic,
		},
		EntryPoint:          "InsertIssues",
		MaxInstances:        1,
		Timeout:             "540s",
		ServiceAccountEmail: serviceAccount,
		SourceUploadUrl:     uploadURL.UploadUrl,
		EnvironmentVariables: map[string]string{
			"JIRA_AUTH_RESOURCE": auth.Resource,
			"JIRA_AUTH_SECRET":   auth.Secret,
			"JIRA_PROJECT":       *jiraProject,
			"SCHEMA_BUCKET":      *googleProject,
			"SCHEMA_PATH":        "schemas/gd_test.json",
			"BIGQUERY_PROJECT":   *googleProject,
			"BIGQUERY_DATASET":   *bigQueryDataset,
			"BIGQUERY_TABLE":     *bigQueryTable,
		},
	}

	log.From(ctx).Info("deploying")
	created, err := fnc.Create(location, function).Context(ctx).Do()
	exists := isExists(err)
	if err != nil && !exists {
		return err
	}

	if exists {
		created, err = fnc.Patch(fmt.Sprintf("%s/functions/%s", location, functionName), function).Context(ctx).Do()
		if err != nil {
			return err
		}
	}

	for !created.Done {
		log.From(ctx).Debug("waiting", zap.Int("seconds", 5))
		time.Sleep(5 * time.Second)
		created, err = svc.Operations.Get(created.Name).Context(ctx).Do()
		if err != nil {
			return err
		}
	}

	fmt.Printf(`
Function: %s

Status:	https://console.cloud.google.com/functions/list?project=%s
Logs:	https://console.cloud.google.com/logs/viewer?project=%s&resource=cloud_function%%2Ffunction_name%%2F%s
Scheduler: https://console.cloud.google.com/cloudscheduler?project=%s
`, functionName, project, project, functionName, project)

	return nil
}

func validateFlags() error {
	if len(*jiraProject) < 1 {
		return errors.New("missing -jiraProject")
	}
	if len(*bigQueryDataset) < 1 {
		return errors.New("missing -bigqueryDataset")
	}
	if len(*bigQueryTable) < 1 {
		return errors.New("missing -bigqueryTable")
	}
	return nil
}

func uploadSchema(ctx context.Context) error {

	if len(*schemaFile) < 1 {
		return errors.New("missing -schemaFile")
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		return err
	}

	obj := client.Bucket(*googleProject).Object(fmt.Sprintf("schemas/%s--%s_%s.json", *jiraProject, *bigQueryDataset, *bigQueryTable))

	log.From(ctx).Info("uploading schema", zap.String("file", *schemaFile), zap.String("destination", fmt.Sprintf("gs://%s/%s", obj.BucketName(), obj.ObjectName())))
	writer := obj.NewWriter(ctx)

	file, err := os.OpenFile(*schemaFile, os.O_RDONLY, os.ModePerm)
	if err != nil {
		return err
	}

	if _, err := io.Copy(writer, file); err != nil {
		return err
	}

	if err := writer.Close(); err != nil {
		return err
	}

	return nil
}

func uploadCode(ctx context.Context, url string) error {
	var body bytes.Buffer

	log.From(ctx).Debug("creating archive")
	archive := zip.NewWriter(&body)

	log.From(ctx).Debug("walking source")
	if err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		ctx := log.WithFields(ctx, zap.String("path", path))
		log.From(ctx).Debug("walking")
		if info.IsDir() {
			path = fmt.Sprintf("%s/", path)
		}

		if strings.HasPrefix(info.Name(), ".") {
			log.From(ctx).Debug("skipping")
			return nil
		}
		if strings.HasPrefix(info.Name(), "util") {
			log.From(ctx).Debug("skipping util")
			return nil
		}

		target, err := archive.Create(path)
		if err != nil {
			return err
		}

		if !info.IsDir() {
			log.From(ctx).Debug("opening file")
			file, err := os.OpenFile(path, os.O_RDONLY, info.Mode())
			if err != nil {
				return err
			}

			log.From(ctx).Debug("copying")
			_, err = io.Copy(target, file)
			file.Close()
			if err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		log.From(ctx).Error("compressing", zap.Error(err))
	}
	log.From(ctx).Debug("closing archive")
	archive.Close()

	log.From(ctx).Debug("creating request")
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, url, &body)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/zip")
	request.Header.Set("x-goog-content-length-range", "0,104857600")

	client := new(http.Client)
	log.From(ctx).Debug("uploading code")
	resp, err := client.Do(request)
	if err != nil {
		return errors.Wrap(err, "uploading code")
	}

	if resp.StatusCode != http.StatusOK {
		fmt.Println(resp)
		return fmt.Errorf("uploading code: status: %v", resp.StatusCode)
	}

	return nil

}

// CreateServiceAccount with the required role bindings and return it's mail
func CreateServiceAccount(ctx context.Context, project string) (string, error) {
	client, err := google.DefaultClient(ctx, iam.CloudPlatformScope)
	if err != nil {
		return "", err
	}

	iamService, err := iam.New(client)
	if err != nil {
		return "", err
	}

	if _, err := iamService.Projects.ServiceAccounts.Create(fmt.Sprintf("projects/%s", project), &iam.CreateServiceAccountRequest{
		AccountId: "jigquery",
		ServiceAccount: &iam.ServiceAccount{
			DisplayName: "jigquery",
		},
	}).Context(ctx).Do(); err != nil && !isExists(err) {
		return "", errors.Wrap(err, "creating service account")
	}

	mail := fmt.Sprintf("%s@%s.iam.gserviceaccount.com", "jigquery", project)

	resourceManager, err := cloudresourcemanager.NewService(ctx)
	if err != nil {
		return "", err
	}

	policy, err := resourceManager.Projects.GetIamPolicy(project, &cloudresourcemanager.GetIamPolicyRequest{}).Context(ctx).Do()
	if err != nil {
		return "", errors.Wrap(err, "fetching policy")
	}

	bindings := []*cloudresourcemanager.Binding{
		{
			Members: []string{fmt.Sprintf("serviceAccount:%s", mail)},
			Role:    "roles/cloudkms.cryptoKeyDecrypter",
		},
		{
			Members: []string{fmt.Sprintf("serviceAccount:%s", mail)},
			Role:    "roles/bigquery.admin",
		},
		{
			Members: []string{fmt.Sprintf("serviceAccount:%s", mail)},
			Role:    "roles/storage.objectViewer",
		},
		{
			Members: []string{fmt.Sprintf("serviceAccount:%s", mail)},
			Role:    "roles/cloudfunctions.serviceAgent",
		},
	}

	policy.Bindings = append(policy.Bindings, bindings...)

	if _, err := resourceManager.Projects.SetIamPolicy(project, &cloudresourcemanager.SetIamPolicyRequest{
		Policy: policy,
	}).Context(ctx).Do(); err != nil {
		return "", errors.Wrap(err, "updating policy")
	}

	return mail, nil
}

func isExists(err error) bool {
	if gerr, ok := err.(*googleapi.Error); ok {
		if gerr.Code == http.StatusConflict {
			return true
		}
	}

	if st, ok := status.FromError(err); ok {
		if st.Code() == codes.AlreadyExists {
			return true
		}
	}

	return false
}
