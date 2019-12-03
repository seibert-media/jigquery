# Jira to BigQuery [JigQuery]

<walkthrough-author name="//SEIBERT/MEDIA GmbH"
repositoryUrl="https://github.com/seibert-media/jigquery">
</walkthrough-author>

## Select a Project

<walkthrough-project-setup></walkthrough-project-setup>

## Prepare Cloud Shell

Press this button, to open the Cloud Shell:
<walkthrough-open-cloud-shell-button/>

Then run the following snippet, to set your Project ID:

```bash
gcloud config set project {{project-id}}
```

## Enable the required APIs

<walkthrough-enable-apis apis="cloudfunctions.googleapis.com,cloudkms.googleapis.com,cloudscheduler.googleapis.com,storage_component,storage_api,bigquery,pubsub"></walkthrough-enable-apis>

## Create your Schema

First, create a basic schema file from the example:

```bash
cp ./.example.schema.json ./.schema.json && cloudshell_open --open_in_editor ./.schema.json
```

This should copy the example and open the resulting file in your Cloud Shell Editor.

## Edit your Schema

The schema defines which fields from your Jira issues get stored in BigQuery.
It consists of a JSON Array containing field definitions, that look like this:

```json
{
  "name": "issue", // name of the field in bigquery
  "type": "string", // data type of the field in bigquery (see Types section)
  "path": "key", // path inside the jira issue in dot-annotation. E.g. fields.updated
  "required": true, // if the field is required in the bigquery schema (optional)
  "repeated": false // if the field is repeated in the bigquery schema (optional)
}
```

You can read more about the schema in the README.md:
<walkthrough-editor-open-file filePath="./README.md"></walkthrough-editor-open-file>

**Note:** Comments are not allowed in JSON, so please do not copy this example without removing them first. A comment is defined like `// comment`.

## Deploy your Function

The following command will ask you several questions regarding your target environment.
Those include:

- Your Jira Project,
- the BigQuery Dataset and Table to store Issues in
- and if not present from a previous execution, your Jira login credentials.

The BigQuery Dataset and Table should not exist, as the Function will create them on it's first run.

**Note:** Your Jira credentials will be encrypted with Google Cloud KMS and eventually deployed to the Cloud Function environment.
They get stored in the same **encrypted** form in your Cloud Shell environment but can only be decrypted by actors with the respective access to your KMS KeyRing and Key.

For encrypting credentials, a KMS Keyring and Key are required. Those will be created if they don't exist (jigquery.jigquery). If you want to provide existing ones, add `--keyring [yourKeyRing] --key [yourKey]` to the call below.

After it was provided with all required information, the program will encrypt your Jira credentials, deploy the Cloud Function and create the respective Pub/Sub and Cloud Scheduler instances to run it once a day.

```bash
GOOGLE_CLOUD_PROJECT={{project-id}} ./cli -mode deploy -i
```
