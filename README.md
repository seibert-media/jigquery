# Sales Metrics Cloud Function

This cloud function stores the issues from a Jira project with the provided schema in Google BigQuery

## Deploy it to your Project

[![Open in Cloud Shell](https://gstatic.com/cloudssh/images/open-btn.svg)](https://ssh.cloud.google.com/cloudshell/editor?cloudshell_git_repo=https%3A%2F%2Fgithub.com%2Fseibert-media%2Fjigquery&cloudshell_open_in_editor=README.md&cloudshell_tutorial=tutorial.md)

## Schema

The schema is defined as a JSON array. The array consists of field definitions, with a single field definition looking like this:

```json
{
  "name": "issue", // name of the field in bigquery
  "type": "string", // data type of the field in bigquery (see Types section)
  "path": "key", // path inside the jira issue in dot-annotation. E.g. fields.updated
  "required": true, // if the field is required in the bigquery schema (optional)
  "repeated": false // if the field is repeated in the bigquery schema (optional)
}
```

### Types

A field can have different kinds in BigQuery, those are [defined by the BigQuery client](https://github.com/googleapis/google-cloud-go/blob/0c193ea4c7649179f7f84a86ed74a788073010a7/bigquery/schema.go#L128):

- STRING
- BYTES
- INTEGER
- FLOAT
- BOOLEAN
- TIMESTAMP

Currently the function is limited to the types above, as some require a custom marshaling to be implemented.

### Path

The path to a field is represented in a dot-annotation.
To find the path for the field you require, check the Jira API.
A good reference is the [Golang implementation of the Issue](https://github.com/andygrunwald/go-jira/blob/1c3507a11eb29b702aad8c6ba27e438b6cd10c93/issue.go#L41).

#### Examples

- `issueKey` -> `key`
- `issue creation date` -> `fields.updated`
- `custom field` -> `fields.customfield_123.value`

### Options

A field in BigQuery can have to additional properties:

- `required`: If this is set to true, the field has to be set when sent to BigQuery
- `repeated`: If this is set to true, the field contains a list of entries that should be added to BigQuery accordingly

## Architecture

The project uses several Google Cloud products to do it's job.

The main work happens in a Google Cloud Function, which creates a BigQuery Dataset and Table (if they don't already exist) based on the Schema.
The Schema gets loaded from Google Cloud Storage.

Then a connection to Jira is made based on credentials encrypted with Google Cloud KMS.

A separate table is being checked for the last time the function executed and the resulting time is used to only fetch Jira issues that updated since.

From those issues, their fields get extracted based on the schema and all resulting entries get streamed into the BigQuery table.
Finally the execution timestamp gets recorded and the function terminates.

## TODOs

- Add an implementation using Jira Webhooks as the current approach does not work well for big numbers of issues due to the Cloud Function timeout.
- Store deployments locally for easy redeployment
- Maybe also deploy Cloud Build Jobs to redeploy on new versions
- Allow setting memory limit
- Only fetch fields from Jira that are in the schema
