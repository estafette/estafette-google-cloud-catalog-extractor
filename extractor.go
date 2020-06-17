package main

import (
	"context"
	"fmt"

	contracts "github.com/estafette/estafette-ci-contracts"
)

type Extractor interface {
	Run(ctx context.Context, organization string) (err error)
}

// NewExtractor returns a new Extractor
func NewExtractor(apiClient ApiClient, googleCloudClient GoogleCloudClient) Extractor {
	return &extractor{
		apiClient:         apiClient,
		googleCloudClient: googleCloudClient,
	}
}

type extractor struct {
	apiClient         ApiClient
	googleCloudClient GoogleCloudClient
}

func (e *extractor) Run(ctx context.Context, organization string) (err error) {

	// initialize extractor
	err = e.init(ctx)
	if err != nil {
		return
	}

	// top level entity (without parent)
	organizationEntity := &contracts.CatalogEntity{
		Key:   organizationKeyName,
		Value: organization,
		Labels: []contracts.Label{
			{
				Key:   organizationKeyName,
				Value: organization,
			},
		},
	}

	// ensure cloud entity exists
	err = e.runCloud(ctx, organizationEntity)
	if err != nil {
		return
	}

	return nil
}

func (e *extractor) init(ctx context.Context) (err error) {
	_, err = e.apiClient.GetToken(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (e *extractor) runCloud(ctx context.Context, organizationEntity *contracts.CatalogEntity) (err error) {

	if organizationEntity.Key != organizationKeyName {
		return fmt.Errorf("Parent key is invalid; %v instead of %v", organizationEntity.Key, organizationKeyName)
	}

	currentClouds, err := e.apiClient.GetCatalogEntities(ctx, organizationEntity.Key, organizationEntity.Value, cloudKeyName)
	if err != nil {
		return err
	}

	desiredClouds := []*contracts.CatalogEntity{
		{
			ParentKey:   organizationEntity.Key,
			ParentValue: organizationEntity.Value,
			Key:         cloudKeyName,
			Value:       cloudKeyValue,
			Labels:      organizationEntity.Labels,
		},
	}

	err = e.syncEntities(ctx, currentClouds, desiredClouds, false)
	if err != nil {
		return err
	}

	for _, dc := range desiredClouds {
		err = e.runProjects(ctx, dc)
		if err != nil {
			return err
		}
	}

	return nil
}

func (e *extractor) runProjects(ctx context.Context, parentEntity *contracts.CatalogEntity) (err error) {

	currentProjects, err := e.apiClient.GetCatalogEntities(ctx, parentEntity.Key, parentEntity.Value, projectKeyName)
	if err != nil {
		return err
	}

	desiredProjects, err := e.googleCloudClient.GetProjects(ctx, parentEntity)
	if err != nil {
		return err
	}

	err = e.syncEntities(ctx, currentProjects, desiredProjects, true)
	if err != nil {
		return err
	}

	return nil
}

func (e *extractor) syncEntities(ctx context.Context, currentEntities []*contracts.CatalogEntity, desiredEntities []*contracts.CatalogEntity, deleteIfNotDesired bool) (err error) {

	for _, de := range desiredEntities {
		isCurrent := false
		for _, cd := range currentEntities {
			if cd.Value == de.Value {
				isCurrent = true
			}
		}
		if !isCurrent {
			// create
			err = e.apiClient.CreateCatalogEntity(ctx, de)
			if err != nil {
				return
			}
		}
	}

	for _, cd := range currentEntities {
		isDesired := false
		for _, de := range desiredEntities {
			if cd.Value == de.Value {
				isDesired = true
				// update
				de.ID = cd.ID
				err = e.apiClient.UpdateCatalogEntity(ctx, de)
				if err != nil {
					return
				}
			}
		}
		if !isDesired && deleteIfNotDesired {
			// delete
			err = e.apiClient.DeleteCatalogEntity(ctx, cd)
			if err != nil {
				return
			}
		}
	}

	return nil
}
