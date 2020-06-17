package main

import (
	"context"

	contracts "github.com/estafette/estafette-ci-contracts"
	"golang.org/x/oauth2/google"
	crmv1 "google.golang.org/api/cloudresourcemanager/v1"
	containerv1 "google.golang.org/api/container/v1"
	iam "google.golang.org/api/iam/v1"
)

type GoogleCloudClient interface {
	GetProjects(ctx context.Context, parentEntity *contracts.CatalogEntity) (projects []*contracts.CatalogEntity, err error)
	GetGKEClusters(ctx context.Context, parentEntity *contracts.CatalogEntity) (clusters []*contracts.CatalogEntity, err error)
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

	return &googleCloudClient{
		crmv1Service:       crmv1Service,
		containerv1Service: containerv1Service,
	}, nil
}

type googleCloudClient struct {
	crmv1Service       *crmv1.Service
	containerv1Service *containerv1.Service
}

func (c *googleCloudClient) GetProjects(ctx context.Context, parentEntity *contracts.CatalogEntity) (projects []*contracts.CatalogEntity, err error) {

	// https://cloud.google.com/resource-manager/reference/rest/v1/projects

	googleProjects := make([]*crmv1.Project, 0)
	nextPageToken := ""

	for {
		// retrieving projects (by page)
		listCall := c.crmv1Service.Projects.List()
		if nextPageToken != "" {
			listCall.PageToken(nextPageToken)
		}

		resp, err := listCall.Do()
		if err != nil {
			return projects, err
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
			Labels:      parentEntity.Labels,
		})
	}

	return
}

func (c *googleCloudClient) GetGKEClusters(ctx context.Context, parentEntity *contracts.CatalogEntity) (clusters []*contracts.CatalogEntity, err error) {

	// https://cloud.google.com/kubernetes-engine/docs/reference/rest/v1/projects.zones.clusters/list

	googleClusters := make([]*containerv1.Cluster, 0)

	listCall := c.containerv1Service.Projects.Zones.Clusters.List(parentEntity.Value, "-")

	resp, err := listCall.Do()
	if err != nil {
		return clusters, err
	}

	googleClusters = append(googleClusters, resp.Clusters...)

	clusters = make([]*contracts.CatalogEntity, 0)
	for _, cluster := range googleClusters {
		clusters = append(clusters, &contracts.CatalogEntity{
			ParentKey:   parentEntity.Key,
			ParentValue: parentEntity.Value,
			Key:         gkeClusterKeyName,
			Value:       cluster.Zone + "/" + cluster.Name,
			Labels:      parentEntity.Labels,
		})
	}

	return
}
