package main

import (
	"context"

	contracts "github.com/estafette/estafette-ci-contracts"
	"golang.org/x/oauth2/google"
	crmv1 "google.golang.org/api/cloudresourcemanager/v1"
	crmv2 "google.golang.org/api/cloudresourcemanager/v2"
	iam "google.golang.org/api/iam/v1"
)

type GoogleCloudClient interface {
	GetProjects(ctx context.Context, parentEntity *contracts.CatalogEntity) (projects []*contracts.CatalogEntity, err error)
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

	crmv2Service, err := crmv2.New(googleClient)
	if err != nil {
		return nil, err
	}

	return &googleCloudClient{
		crmv1Service: crmv1Service,
		crmv2Service: crmv2Service,
	}, nil
}

type googleCloudClient struct {
	crmv1Service *crmv1.Service
	crmv2Service *crmv2.Service
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
