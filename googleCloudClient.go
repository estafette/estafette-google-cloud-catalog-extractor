package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	contracts "github.com/estafette/estafette-ci-contracts"
	foundation "github.com/estafette/estafette-foundation"
	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2/google"
	bigqueryv2 "google.golang.org/api/bigquery/v2"
	bigtableadminv2 "google.golang.org/api/bigtableadmin/v2"
	cloudfunctionsv1 "google.golang.org/api/cloudfunctions/v1"
	crmv1 "google.golang.org/api/cloudresourcemanager/v1"
	containerv1 "google.golang.org/api/container/v1"
	dataflowv1b3 "google.golang.org/api/dataflow/v1b3"
	datastorev1 "google.golang.org/api/datastore/v1"
	"google.golang.org/api/googleapi"
	iam "google.golang.org/api/iam/v1"
	pubsubv1 "google.golang.org/api/pubsub/v1"
	sqlv1beta4 "google.golang.org/api/sql/v1beta4"
	storagev1 "google.golang.org/api/storage/v1"
)

var (
	// ErrAPINotEnabled is returned when an api is not enabled
	ErrAPINotEnabled = errors.New("The api is not enabled")

	// ErrUnknownProjectID is returned when an api throws 'googleapi: Error 400: Unknown project id: 0, invalid'
	ErrUnknownProjectID = errors.New("The project id is unknown")

	// ErrProjectNotFound is returned when an api throws 'googleapi: Error 404: The requested project was not found., notFound'
	ErrProjectNotFound = errors.New("The project is not found")

	// ErrEntityNotFound is returned when pubsub topics return htlm with a 404
	ErrEntityNotFound = errors.New("Entity is not found")
)

type GoogleCloudClient interface {
	GetProjects(ctx context.Context, parentEntity *contracts.CatalogEntity) (projects []*contracts.CatalogEntity, err error)
	GetGKEClusters(ctx context.Context, parentEntity *contracts.CatalogEntity) (clusters []*contracts.CatalogEntity, err error)
	GetPubSubTopics(ctx context.Context, parentEntity *contracts.CatalogEntity) (topics []*contracts.CatalogEntity, err error)
	GetCloudFunctions(ctx context.Context, parentEntity *contracts.CatalogEntity) (cloudfunctions []*contracts.CatalogEntity, err error)
	GetStorageBuckets(ctx context.Context, parentEntity *contracts.CatalogEntity) (buckets []*contracts.CatalogEntity, err error)
	GetDataflowJobs(ctx context.Context, parentEntity *contracts.CatalogEntity) (jobs []*contracts.CatalogEntity, err error)
	GetBigqueryDatasets(ctx context.Context, parentEntity *contracts.CatalogEntity) (datasets []*contracts.CatalogEntity, err error)
	GetBigqueryTables(ctx context.Context, parentEntity *contracts.CatalogEntity) (tables []*contracts.CatalogEntity, err error)
	GetCloudSQLDatabaseInstances(ctx context.Context, parentEntity *contracts.CatalogEntity) (instances []*contracts.CatalogEntity, err error)
	GetCloudSQLDatabases(ctx context.Context, parentEntity *contracts.CatalogEntity) (databases []*contracts.CatalogEntity, err error)
	GetBigTableInstances(ctx context.Context, parentEntity *contracts.CatalogEntity) (instances []*contracts.CatalogEntity, err error)
	GetBigTableClusters(ctx context.Context, parentEntity *contracts.CatalogEntity) (clusters []*contracts.CatalogEntity, err error)
}

// NewGoogleCloudClient returns a new GoogleCloudClient
func NewGoogleCloudClient(ctx context.Context) (GoogleCloudClient, error) {

	// use service account to authenticate against gcp apis
	googleClient, err := google.DefaultClient(ctx, iam.CloudPlatformScope)
	if err != nil {
		return nil, err
	}

	crmv1Service, err := crmv1.New(googleClient)
	if err != nil {
		return nil, err
	}

	containerv1Service, err := containerv1.New(googleClient)
	if err != nil {
		return nil, err
	}

	pubsubv1Service, err := pubsubv1.New(googleClient)
	if err != nil {
		return nil, err
	}

	cloudfunctionsv1Service, err := cloudfunctionsv1.New(googleClient)
	if err != nil {
		return nil, err
	}

	dataflowv1b3Service, err := dataflowv1b3.New(googleClient)
	if err != nil {
		return nil, err
	}

	storagev1Service, err := storagev1.New(googleClient)
	if err != nil {
		return nil, err
	}

	bigqueryv2Service, err := bigqueryv2.New(googleClient)
	if err != nil {
		return nil, err
	}

	sqlv1beta4Service, err := sqlv1beta4.New(googleClient)
	if err != nil {
		return nil, err
	}

	datastorev1Service, err := datastorev1.New(googleClient)
	if err != nil {
		return nil, err
	}
	bigtableadminv2Service, err := bigtableadminv2.New(googleClient)

	return &googleCloudClient{
		crmv1Service:            crmv1Service,
		containerv1Service:      containerv1Service,
		pubsubv1Service:         pubsubv1Service,
		cloudfunctionsv1Service: cloudfunctionsv1Service,
		dataflowv1b3Service:     dataflowv1b3Service,
		storagev1Service:        storagev1Service,
		bigqueryv2Service:       bigqueryv2Service,
		sqlv1beta4Service:       sqlv1beta4Service,
		datastorev1Service:      datastorev1Service,
		bigtableadminv2Service:  bigtableadminv2Service,
	}, nil
}

type googleCloudClient struct {
	crmv1Service            *crmv1.Service
	containerv1Service      *containerv1.Service
	pubsubv1Service         *pubsubv1.Service
	cloudfunctionsv1Service *cloudfunctionsv1.Service
	dataflowv1b3Service     *dataflowv1b3.Service
	storagev1Service        *storagev1.Service
	bigqueryv2Service       *bigqueryv2.Service
	sqlv1beta4Service       *sqlv1beta4.Service
	datastorev1Service      *datastorev1.Service
	bigtableadminv2Service  *bigtableadminv2.Service
}

func (c *googleCloudClient) GetProjects(ctx context.Context, parentEntity *contracts.CatalogEntity) (projects []*contracts.CatalogEntity, err error) {

	// https://cloud.google.com/resource-manager/reference/rest/v1/projects

	log.Debug().Msg("Retrieving Google Cloud projects")

	googleProjects := make([]*crmv1.Project, 0)
	nextPageToken := ""

	for {
		// retrieving projects (by page)
		var resp *crmv1.ListProjectsResponse
		err = foundation.Retry(func() error {
			listCall := c.crmv1Service.Projects.List()
			if nextPageToken != "" {
				listCall.PageToken(nextPageToken)
			}

			resp, err = listCall.Do()
			if err != nil {
				return err
			}
			return nil
		}, c.isRetryableErrorCustomOption(), foundation.LastErrorOnly(true))
		if err != nil {
			return c.substituteErrorsToIgnore(projects, err)
		}

		googleProjects = append(googleProjects, resp.Projects...)

		if resp.NextPageToken == "" {
			break
		}
		nextPageToken = resp.NextPageToken
	}

	projects = make([]*contracts.CatalogEntity, 0)
	for _, p := range googleProjects {
		projects = append(projects, &contracts.CatalogEntity{
			ParentKey:   parentEntity.Key,
			ParentValue: parentEntity.Value,
			Key:         projectKeyName,
			Value:       p.ProjectId,
			Labels: append(parentEntity.Labels, contracts.Label{
				Key:   projectKeyName,
				Value: p.ProjectId,
			}),
		})
	}

	return
}

func (c *googleCloudClient) GetGKEClusters(ctx context.Context, parentEntity *contracts.CatalogEntity) (clusters []*contracts.CatalogEntity, err error) {

	// https://cloud.google.com/kubernetes-engine/docs/reference/rest/v1/projects.zones.clusters/list

	log.Debug().Msgf("Retrieving GKE clusters for project %v", parentEntity.Value)

	googleClusters := make([]*containerv1.Cluster, 0)

	var resp *containerv1.ListClustersResponse
	err = foundation.Retry(func() error {
		listCall := c.containerv1Service.Projects.Zones.Clusters.List(parentEntity.Value, "-")

		resp, err = listCall.Do()
		if err != nil {
			return err
		}
		return nil
	}, c.isRetryableErrorCustomOption(), foundation.LastErrorOnly(true))
	if err != nil {
		return c.substituteErrorsToIgnore(clusters, err)
	}

	googleClusters = append(googleClusters, resp.Clusters...)

	clusters = make([]*contracts.CatalogEntity, 0)
	for _, cluster := range googleClusters {
		clusters = append(clusters, &contracts.CatalogEntity{
			ParentKey:   parentEntity.Key,
			ParentValue: parentEntity.Value,
			Key:         gkeClusterKeyName,
			Value:       cluster.Location + "/" + cluster.Name,
			Labels: append(parentEntity.Labels, contracts.Label{
				Key:   gkeClusterKeyName,
				Value: cluster.Location + "/" + cluster.Name,
			}, contracts.Label{
				Key:   locationLabelKey,
				Value: cluster.Location,
			}),
		})
	}

	return
}

func (c *googleCloudClient) GetPubSubTopics(ctx context.Context, parentEntity *contracts.CatalogEntity) (topics []*contracts.CatalogEntity, err error) {

	// https://cloud.google.com/pubsub/docs/reference/rest/v1/projects.topics/list

	log.Debug().Msgf("Retrieving Pub/Sub topics for project %v", parentEntity.Value)

	googlePubsubTopics := make([]*pubsubv1.Topic, 0)
	nextPageToken := ""

	for {
		// retrieving pubsub topics (by page)
		var resp *pubsubv1.ListTopicsResponse
		err = foundation.Retry(func() error {
			listCall := c.pubsubv1Service.Projects.Topics.List(parentEntity.Value)
			if nextPageToken != "" {
				listCall.PageToken(nextPageToken)
			}

			resp, err = listCall.Do()
			if err != nil {
				return err
			}
			return nil
		}, c.isRetryableErrorCustomOption(), foundation.LastErrorOnly(true))
		if err != nil {
			return c.substituteErrorsToIgnore(topics, err)
		}

		googlePubsubTopics = append(googlePubsubTopics, resp.Topics...)

		if resp.NextPageToken == "" {
			break
		}
		nextPageToken = resp.NextPageToken
	}

	topics = make([]*contracts.CatalogEntity, 0)
	for _, topic := range googlePubsubTopics {
		topics = append(topics, &contracts.CatalogEntity{
			ParentKey:   parentEntity.Key,
			ParentValue: parentEntity.Value,
			Key:         pubsubTopicKeyName,
			Value:       topic.Name,
			Labels: append(parentEntity.Labels, contracts.Label{
				Key:   pubsubTopicKeyName,
				Value: topic.Name,
			}),
		})
	}

	return
}

func (c *googleCloudClient) GetCloudFunctions(ctx context.Context, parentEntity *contracts.CatalogEntity) (cloudfunctions []*contracts.CatalogEntity, err error) {

	log.Debug().Msgf("Retrieving Google Cloud Functions for project %v", parentEntity.Value)

	googleCloudFunctions := make([]*cloudfunctionsv1.CloudFunction, 0)
	nextPageToken := ""

	for {
		// retrieving cloud functions (by page)
		var resp *cloudfunctionsv1.ListFunctionsResponse
		err = foundation.Retry(func() error {
			listCall := c.cloudfunctionsv1Service.Projects.Locations.Functions.List(fmt.Sprintf("projects/%v/locations/-", parentEntity.Value))
			if nextPageToken != "" {
				listCall.PageToken(nextPageToken)
			}

			resp, err = listCall.Do()
			if err != nil {
				return err
			}
			return nil
		}, c.isRetryableErrorCustomOption(), foundation.LastErrorOnly(true))
		if err != nil {
			return c.substituteErrorsToIgnore(cloudfunctions, err)
		}

		googleCloudFunctions = append(googleCloudFunctions, resp.Functions...)

		if resp.NextPageToken == "" {
			break
		}
		nextPageToken = resp.NextPageToken
	}

	cloudfunctions = make([]*contracts.CatalogEntity, 0)
	for _, cloudfunction := range googleCloudFunctions {

		name := strings.TrimPrefix(cloudfunction.Name, fmt.Sprintf("projects/%v/locations/", parentEntity.Value))
		name = strings.Replace(name, "/functions/", "/", 1)

		cloudfunctions = append(cloudfunctions, &contracts.CatalogEntity{
			ParentKey:   parentEntity.Key,
			ParentValue: parentEntity.Value,
			Key:         cloudfunctionKeyName,
			Value:       name,
			Labels: append(parentEntity.Labels, contracts.Label{
				Key:   cloudfunctionKeyName,
				Value: name,
			}),
		})
	}

	return
}

func (c *googleCloudClient) GetStorageBuckets(ctx context.Context, parentEntity *contracts.CatalogEntity) (buckets []*contracts.CatalogEntity, err error) {

	log.Debug().Msgf("Retrieving Google Cloud storage buckets for project %v", parentEntity.Value)

	googleStorageBuckets := make([]*storagev1.Bucket, 0)
	nextPageToken := ""

	for {
		// retrieving storage buckets (by page)
		var resp *storagev1.Buckets
		err = foundation.Retry(func() error {
			listCall := c.storagev1Service.Buckets.List(parentEntity.Value)
			if nextPageToken != "" {
				listCall.PageToken(nextPageToken)
			}

			resp, err = listCall.Do()
			if err != nil {
				return err
			}
			return nil
		}, c.isRetryableErrorCustomOption(), foundation.LastErrorOnly(true))
		if err != nil {
			return c.substituteErrorsToIgnore(buckets, err)
		}

		googleStorageBuckets = append(googleStorageBuckets, resp.Items...)

		if resp.NextPageToken == "" {
			break
		}
		nextPageToken = resp.NextPageToken
	}

	buckets = make([]*contracts.CatalogEntity, 0)
	for _, bucket := range googleStorageBuckets {
		buckets = append(buckets, &contracts.CatalogEntity{
			ParentKey:   parentEntity.Key,
			ParentValue: parentEntity.Value,
			Key:         storageBucketKeyName,
			Value:       bucket.Name,
			Labels: append(parentEntity.Labels, contracts.Label{
				Key:   storageBucketKeyName,
				Value: bucket.Name,
			}, contracts.Label{
				Key:   locationLabelKey,
				Value: bucket.Location,
			}),
		})
	}

	return
}

func (c *googleCloudClient) GetDataflowJobs(ctx context.Context, parentEntity *contracts.CatalogEntity) (jobs []*contracts.CatalogEntity, err error) {

	log.Debug().Msgf("Retrieving Google Cloud Dataflow jobs for project %v", parentEntity.Value)

	googleDataflowJobs := make([]*dataflowv1b3.Job, 0)
	nextPageToken := ""

	for {
		// retrieving dataflow jobs (by page)
		var resp *dataflowv1b3.ListJobsResponse
		err = foundation.Retry(func() error {
			listCall := c.dataflowv1b3Service.Projects.Jobs.List(parentEntity.Value)
			if nextPageToken != "" {
				listCall.PageToken(nextPageToken)
			}

			resp, err = listCall.Do()
			if err != nil {
				return err
			}
			return nil
		}, c.isRetryableErrorCustomOption(), foundation.LastErrorOnly(true))
		if err != nil {
			return c.substituteErrorsToIgnore(jobs, err)
		}

		googleDataflowJobs = append(googleDataflowJobs, resp.Jobs...)

		if resp.NextPageToken == "" {
			break
		}
		nextPageToken = resp.NextPageToken
	}

	jobs = make([]*contracts.CatalogEntity, 0)
	for _, job := range googleDataflowJobs {
		jobs = append(jobs, &contracts.CatalogEntity{
			ParentKey:   parentEntity.Key,
			ParentValue: parentEntity.Value,
			Key:         dataflowJobKeyName,
			Value:       job.Name,
			Labels: append(parentEntity.Labels, contracts.Label{
				Key:   dataflowJobKeyName,
				Value: job.Name,
			}),
		})
	}

	return
}

func (c *googleCloudClient) GetBigqueryDatasets(ctx context.Context, parentEntity *contracts.CatalogEntity) (datasets []*contracts.CatalogEntity, err error) {
	log.Debug().Msgf("Retrieving BigQuery datasets for project %v", parentEntity.Value)

	googleBigqueryDatasets := make([]*bigqueryv2.DatasetListDatasets, 0)
	nextPageToken := ""

	for {
		// retrieving bigquery datasets (by page)
		var resp *bigqueryv2.DatasetList
		err = foundation.Retry(func() error {
			listCall := c.bigqueryv2Service.Datasets.List(parentEntity.Value)
			if nextPageToken != "" {
				listCall.PageToken(nextPageToken)
			}

			resp, err = listCall.Do()
			if err != nil {
				return err
			}
			return nil
		}, c.isRetryableErrorCustomOption(), foundation.LastErrorOnly(true))
		if err != nil {
			return c.substituteErrorsToIgnore(datasets, err)
		}

		googleBigqueryDatasets = append(googleBigqueryDatasets, resp.Datasets...)

		if resp.NextPageToken == "" {
			break
		}
		nextPageToken = resp.NextPageToken
	}

	datasets = make([]*contracts.CatalogEntity, 0)
	for _, dataset := range googleBigqueryDatasets {
		datasetIDParts := strings.Split(dataset.Id, ":")
		if len(datasetIDParts) == 0 {
			return datasets, fmt.Errorf("Dataset id '%v' is incorrect", dataset.Id)
		}
		datasetID := datasetIDParts[len(datasetIDParts)-1]

		datasets = append(datasets, &contracts.CatalogEntity{
			ParentKey:   parentEntity.Key,
			ParentValue: parentEntity.Value,
			Key:         bigqueryDatasetKeyName,
			Value:       datasetID,
			Labels: append(parentEntity.Labels, contracts.Label{
				Key:   bigqueryDatasetKeyName,
				Value: datasetID,
			}, contracts.Label{
				Key:   locationLabelKey,
				Value: dataset.Location,
			}),
		})
	}

	return
}

func (c *googleCloudClient) GetBigqueryTables(ctx context.Context, parentEntity *contracts.CatalogEntity) (tables []*contracts.CatalogEntity, err error) {
	log.Debug().Msgf("Retrieving BigQuery tables for project %v and dataset %v", parentEntity.ParentValue, parentEntity.Value)

	googleBigqueryTables := make([]*bigqueryv2.TableListTables, 0)
	nextPageToken := ""

	for {
		// retrieving bigquery tables (by page)
		var resp *bigqueryv2.TableList
		err = foundation.Retry(func() error {
			listCall := c.bigqueryv2Service.Tables.List(parentEntity.ParentValue, parentEntity.Value)
			if nextPageToken != "" {
				listCall.PageToken(nextPageToken)
			}

			resp, err = listCall.Do()
			if err != nil {
				return err
			}
			return nil
		}, c.isRetryableErrorCustomOption(), foundation.LastErrorOnly(true))
		if err != nil {
			return c.substituteErrorsToIgnore(tables, err)
		}

		googleBigqueryTables = append(googleBigqueryTables, resp.Tables...)

		if resp.NextPageToken == "" {
			break
		}
		nextPageToken = resp.NextPageToken
	}

	tables = make([]*contracts.CatalogEntity, 0)
	for _, table := range googleBigqueryTables {
		tableIDParts := strings.Split(table.Id, ":")
		if len(tableIDParts) == 0 {
			return tables, fmt.Errorf("Table id '%v' is incorrect", table.Id)
		}
		tableID := tableIDParts[len(tableIDParts)-1]

		tables = append(tables, &contracts.CatalogEntity{
			ParentKey:   parentEntity.Key,
			ParentValue: parentEntity.Value,
			Key:         bigqueryTableKeyName,
			Value:       tableID,
			Labels: append(parentEntity.Labels, contracts.Label{
				Key:   bigqueryTableKeyName,
				Value: tableID,
			}),
		})
	}

	return
}

func (c *googleCloudClient) GetCloudSQLDatabaseInstances(ctx context.Context, parentEntity *contracts.CatalogEntity) (instances []*contracts.CatalogEntity, err error) {
	log.Debug().Msgf("Retrieving Cloud SQL instances for project %v", parentEntity.Value)

	googleCloudSQLInstances := make([]*sqlv1beta4.DatabaseInstance, 0)
	nextPageToken := ""

	for {
		// retrieving cloud sql instances (by page)
		var resp *sqlv1beta4.InstancesListResponse
		err = foundation.Retry(func() error {
			listCall := c.sqlv1beta4Service.Instances.List(parentEntity.Value)
			if nextPageToken != "" {
				listCall.PageToken(nextPageToken)
			}

			resp, err = listCall.Do()
			if err != nil {
				return err
			}

			return nil
		}, c.isRetryableErrorCustomOption(), foundation.LastErrorOnly(true))
		if err != nil {
			return c.substituteErrorsToIgnore(instances, err)
		}

		googleCloudSQLInstances = append(googleCloudSQLInstances, resp.Items...)

		if resp.NextPageToken == "" {
			break
		}
		nextPageToken = resp.NextPageToken
	}

	instances = make([]*contracts.CatalogEntity, 0)
	for _, instance := range googleCloudSQLInstances {
		instances = append(instances, &contracts.CatalogEntity{
			ParentKey:   parentEntity.Key,
			ParentValue: parentEntity.Value,
			Key:         cloudsqlInstanceKeyName,
			Value:       instance.Name,
			Labels: append(parentEntity.Labels, contracts.Label{
				Key:   cloudsqlInstanceKeyName,
				Value: instance.Name,
			}, contracts.Label{
				Key:   locationLabelKey,
				Value: instance.Region,
			}),
		})
	}

	return
}

func (c *googleCloudClient) GetCloudSQLDatabases(ctx context.Context, parentEntity *contracts.CatalogEntity) (databases []*contracts.CatalogEntity, err error) {
	log.Debug().Msgf("Retrieving Cloud SQL databases for project %v and instance %v", parentEntity.ParentValue, parentEntity.Value)

	googleCloudSQLDatabases := make([]*sqlv1beta4.Database, 0)

	var resp *sqlv1beta4.DatabasesListResponse
	err = foundation.Retry(func() error {
		listCall := c.sqlv1beta4Service.Databases.List(parentEntity.ParentValue, parentEntity.Value)
		resp, err = listCall.Do()
		if err != nil {
			return err
		}

		return nil
	}, c.isRetryableErrorCustomOption(), foundation.LastErrorOnly(true))
	if err != nil {
		return c.substituteErrorsToIgnore(databases, err)
	}

	googleCloudSQLDatabases = append(googleCloudSQLDatabases, resp.Items...)

	databases = make([]*contracts.CatalogEntity, 0)
	for _, database := range googleCloudSQLDatabases {
		databases = append(databases, &contracts.CatalogEntity{
			ParentKey:   parentEntity.Key,
			ParentValue: parentEntity.Value,
			Key:         cloudsqlDatabaseKeyName,
			Value:       database.Name,
			Labels: append(parentEntity.Labels, contracts.Label{
				Key:   cloudsqlDatabaseKeyName,
				Value: database.Name,
			}),
		})
	}

	return
}

func (c *googleCloudClient) GetBigTableInstances(ctx context.Context, parentEntity *contracts.CatalogEntity) (instances []*contracts.CatalogEntity, err error) {
	log.Debug().Msgf("Retrieving BigTable instances for project %v", parentEntity.Value)

	googleBigTableInstances := make([]*bigtableadminv2.Instance, 0)
	nextPageToken := ""

	for {
		// retrieving cloud sql instances (by page)
		var resp *bigtableadminv2.ListInstancesResponse
		err = foundation.Retry(func() error {
			listCall := c.bigtableadminv2Service.Projects.Instances.List(fmt.Sprintf("projects/%v", parentEntity.Value))
			if nextPageToken != "" {
				listCall.PageToken(nextPageToken)
			}

			resp, err = listCall.Do()
			if err != nil {
				return err
			}

			return nil
		}, c.isRetryableErrorCustomOption(), foundation.LastErrorOnly(true))
		if err != nil {
			return c.substituteErrorsToIgnore(instances, err)
		}

		googleBigTableInstances = append(googleBigTableInstances, resp.Instances...)

		if resp.NextPageToken == "" {
			break
		}
		nextPageToken = resp.NextPageToken
	}

	instances = make([]*contracts.CatalogEntity, 0)
	for _, instance := range googleBigTableInstances {
		// projects/{project}/instances/a-z+[a-z0-9]
		instanceNameParts := strings.Split(instance.Name, "/")
		if len(instanceNameParts) == 0 {
			return instances, fmt.Errorf("Bigtable instance name '%v' is incorrect", instance.Name)
		}
		instanceName := instanceNameParts[len(instanceNameParts)-1]

		instances = append(instances, &contracts.CatalogEntity{
			ParentKey:   parentEntity.Key,
			ParentValue: parentEntity.Value,
			Key:         bigtableInstanceKeyName,
			Value:       instanceName,
			Labels: append(parentEntity.Labels, contracts.Label{
				Key:   bigtableInstanceKeyName,
				Value: instanceName,
			}),
		})
	}

	return
}

func (c *googleCloudClient) GetBigTableClusters(ctx context.Context, parentEntity *contracts.CatalogEntity) (clusters []*contracts.CatalogEntity, err error) {
	log.Debug().Msgf("Retrieving BigTable clusters for project %v and instance %v", parentEntity.ParentValue, parentEntity.Value)

	googleBigTableClusters := make([]*bigtableadminv2.Cluster, 0)
	nextPageToken := ""

	for {
		// retrieving cloud sql instances (by page)
		var resp *bigtableadminv2.ListClustersResponse
		err = foundation.Retry(func() error {
			listCall := c.bigtableadminv2Service.Projects.Instances.Clusters.List(fmt.Sprintf("projects/%v/instances/%v", parentEntity.ParentValue, parentEntity.Value))
			if nextPageToken != "" {
				listCall.PageToken(nextPageToken)
			}

			resp, err = listCall.Do()
			if err != nil {
				return err
			}

			return nil
		}, c.isRetryableErrorCustomOption(), foundation.LastErrorOnly(true))
		if err != nil {
			return c.substituteErrorsToIgnore(clusters, err)
		}

		googleBigTableClusters = append(googleBigTableClusters, resp.Clusters...)

		if resp.NextPageToken == "" {
			break
		}
		nextPageToken = resp.NextPageToken
	}

	clusters = make([]*contracts.CatalogEntity, 0)
	for _, cluster := range googleBigTableClusters {

		// projects/{project}/instances/{instance}/clusters/a-z*
		clusterNameParts := strings.Split(cluster.Name, "/")
		if len(clusterNameParts) == 0 {
			return clusters, fmt.Errorf("Bigtable cluster name '%v' is incorrect", cluster.Name)
		}
		clusterName := clusterNameParts[len(clusterNameParts)-1]

		clusters = append(clusters, &contracts.CatalogEntity{
			ParentKey:   parentEntity.Key,
			ParentValue: parentEntity.Value,
			Key:         bigtableClusterKeyName,
			Value:       clusterName,
			Labels: append(parentEntity.Labels, contracts.Label{
				Key:   bigtableClusterKeyName,
				Value: clusterName,
			}, contracts.Label{
				Key:   locationLabelKey,
				Value: cluster.Location,
			}),
		})
	}

	return
}

func (c *googleCloudClient) substituteErrorsToIgnore(entities []*contracts.CatalogEntity, err error) ([]*contracts.CatalogEntity, error) {
	if googleapiErr, ok := err.(*googleapi.Error); ok && googleapiErr.Code == http.StatusForbidden {
		return entities, ErrAPINotEnabled
	}
	if googleapiErr, ok := err.(*googleapi.Error); ok && googleapiErr.Code == http.StatusBadRequest && err.Error() == "googleapi: Error 400: Unknown project id: 0, invalid" {
		return entities, ErrUnknownProjectID
	}
	if googleapiErr, ok := err.(*googleapi.Error); ok && googleapiErr.Code == http.StatusBadRequest && strings.HasSuffix(err.Error(), "has not enabled BigQuery., invalid") {
		return entities, ErrAPINotEnabled
	}
	if googleapiErr, ok := err.(*googleapi.Error); ok && googleapiErr.Code == http.StatusNotFound && err.Error() == "googleapi: Error 404: The requested project was not found., notFound" {
		return entities, ErrProjectNotFound
	}
	if googleapiErr, ok := err.(*googleapi.Error); ok && googleapiErr.Code == http.StatusNotFound {
		return entities, ErrEntityNotFound
	}

	return entities, err
}

func (c *googleCloudClient) isRetryableErrorCustomOption() foundation.RetryOption {
	return func(c *foundation.RetryConfig) {
		c.IsRetryableError = func(err error) bool {
			switch e := err.(type) {
			case *googleapi.Error:
				// Retry on 429 and 5xx, according to
				// https://cloud.google.com/storage/docs/exponential-backoff.
				return e.Code == 429 || (e.Code >= 500 && e.Code < 600)
			case *url.Error:
				// Retry socket-level errors ECONNREFUSED and ENETUNREACH (from syscall).
				// Unfortunately the error type is unexported, so we resort to string
				// matching.
				retriable := []string{"connection refused", "connection reset"}
				for _, s := range retriable {
					if strings.Contains(e.Error(), s) {
						return true
					}
				}
				return false
			case interface{ Temporary() bool }:
				return e.Temporary()
			default:
				return false
			}
		}
	}
}
