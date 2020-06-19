package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	contracts "github.com/estafette/estafette-ci-contracts"
	"github.com/rs/zerolog/log"
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
	err = e.runClouds(ctx, organizationEntity)
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

func (e *extractor) runClouds(ctx context.Context, parentEntity *contracts.CatalogEntity) (err error) {

	if parentEntity.Key != organizationKeyName {
		return fmt.Errorf("Parent key is invalid; %v instead of %v", parentEntity.Key, organizationKeyName)
	}

	currentClouds, err := e.apiClient.GetCatalogEntities(ctx, parentEntity.Key, parentEntity.Value, cloudKeyName)
	if err != nil {
		return err
	}

	desiredClouds := []*contracts.CatalogEntity{
		{
			ParentKey:   parentEntity.Key,
			ParentValue: parentEntity.Value,
			Key:         cloudKeyName,
			Value:       cloudKeyValue,
			Labels: append(parentEntity.Labels, contracts.Label{
				Key:   cloudKeyName,
				Value: cloudKeyValue,
			}),
		},
	}

	err = e.syncEntities(ctx, currentClouds, desiredClouds, false)
	if err != nil {
		return err
	}

	// fetch projects for each cloud
	err = e.loopEntitiesInParallel(ctx, 10, desiredClouds, func(ctx context.Context, entity *contracts.CatalogEntity) error {
		return e.runProjects(ctx, entity)
	})
	if err != nil {
		return err
	}

	return nil
}

func (e *extractor) runProjects(ctx context.Context, parentEntity *contracts.CatalogEntity) (err error) {

	if parentEntity.Key != cloudKeyName {
		return fmt.Errorf("Parent key is invalid; %v instead of %v", parentEntity.Key, cloudKeyName)
	}

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

	// fetch gke clusters for each project
	err = e.loopEntitiesInParallel(ctx, 5, desiredProjects, func(ctx context.Context, entity *contracts.CatalogEntity) error {
		return e.runFunction(ctx, projectKeyName, gkeClusterKeyName, entity, e.googleCloudClient.GetGKEClusters, true)
	})
	if err != nil {
		return err
	}

	// // fetch pubsub topics for each project
	// err = e.loopEntitiesInParallel(ctx, 5, desiredProjects, func(ctx context.Context, entity *contracts.CatalogEntity) error {
	// 	return e.runFunction(ctx, projectKeyName, pubsubTopicKeyName, entity, e.googleCloudClient.GetPubSubTopics, true)
	// })
	// if err != nil {
	// 	return err
	// }

	// fetch cloud functions for each project
	err = e.loopEntitiesInParallel(ctx, 5, desiredProjects, func(ctx context.Context, entity *contracts.CatalogEntity) error {
		return e.runFunction(ctx, projectKeyName, cloudfunctionKeyName, entity, e.googleCloudClient.GetCloudFunctions, true)
	})
	if err != nil {
		return err
	}

	// fetch storage buckets for each project
	err = e.loopEntitiesInParallel(ctx, 5, desiredProjects, func(ctx context.Context, entity *contracts.CatalogEntity) error {
		return e.runFunction(ctx, projectKeyName, storageBucketKeyName, entity, e.googleCloudClient.GetStorageBuckets, true)
	})
	if err != nil {
		return err
	}

	// fetch dataflow jobs for each project
	err = e.loopEntitiesInParallel(ctx, 5, desiredProjects, func(ctx context.Context, entity *contracts.CatalogEntity) error {
		return e.runFunction(ctx, projectKeyName, dataflowJobKeyName, entity, e.googleCloudClient.GetDataflowJobs, true)
	})
	if err != nil {
		return err
	}

	// fetch bigquery datasets for each project
	err = e.loopEntitiesInParallel(ctx, 5, desiredProjects, func(ctx context.Context, entity *contracts.CatalogEntity) error {
		return e.runFunction(ctx, projectKeyName, bigqueryDatasetKeyName, entity, e.googleCloudClient.GetBigqueryDatasets, true)
	})
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
				break
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

				if e.entitiesAreEqual(cd, de) {
					// log.Debug().Msg("Current and desired entity are equal, skipping updated")
					break
				}

				// update
				de.ID = cd.ID
				err = e.apiClient.UpdateCatalogEntity(ctx, de)
				if err != nil {
					return
				}

				break
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

func (e *extractor) runFunction(ctx context.Context, acceptedParentKeyName, currentEntityKeyName string, parentEntity *contracts.CatalogEntity, desiredEntitiesFunc func(ctx context.Context, parentEntity *contracts.CatalogEntity) (desiredEntities []*contracts.CatalogEntity, err error), deleteIfNotDesired bool) (err error) {

	if parentEntity.Key != acceptedParentKeyName {
		return fmt.Errorf("Parent key is invalid; %v instead of %v", parentEntity.Key, acceptedParentKeyName)
	}

	desiredEntities, err := desiredEntitiesFunc(ctx, parentEntity)
	if err != nil {
		if errors.Is(err, ErrAPINotEnabled) || errors.Is(err, ErrUnknownProjectID) {
			// ignoring api is not enabled errors and continueing, since if the api is disabled the parent will not have resources of that type anyway
			return nil
		}

		log.Warn().Err(err).Msgf("Failed retrieving desired entities of type %v", currentEntityKeyName)

		return err
	}

	currentEntities, err := e.apiClient.GetCatalogEntities(ctx, parentEntity.Key, parentEntity.Value, currentEntityKeyName)
	if err != nil {
		return err
	}

	err = e.syncEntities(ctx, currentEntities, desiredEntities, deleteIfNotDesired)
	if err != nil {
		return err
	}

	return nil
}

func (e *extractor) loopEntitiesInParallel(ctx context.Context, concurrency int, entities []*contracts.CatalogEntity, runFunction func(ctx context.Context, entity *contracts.CatalogEntity) error) (err error) {
	// http://jmoiron.net/blog/limiting-concurrency-in-go/
	semaphore := make(chan bool, concurrency)

	resultChannel := make(chan error, len(entities))

	for _, entity := range entities {
		// try to fill semaphore up to it's full size otherwise wait for a routine to finish
		semaphore <- true

		go func(ctx context.Context, entity *contracts.CatalogEntity) {
			// lower semaphore once the routine's finished, making room for another one to start
			defer func() { <-semaphore }()
			resultChannel <- runFunction(ctx, entity)
		}(ctx, entity)
	}

	// try to fill semaphore up to it's full size which only succeeds if all routines have finished
	for i := 0; i < cap(semaphore); i++ {
		semaphore <- true
	}

	close(resultChannel)
	for err := range resultChannel {
		if err != nil {
			return err
		}
	}

	return nil
}

func (e *extractor) entitiesAreEqual(currentEntity *contracts.CatalogEntity, desiredEntity *contracts.CatalogEntity) bool {

	// id, parent key, parent value, key and value are immutable so no need to check those
	// log.Debug().Interface("currentEntity", *currentEntity).Interface("desiredEntity", *desiredEntity).Msg("Comparing equality of entities")

	// compare LinkedPipeline
	if currentEntity.LinkedPipeline != desiredEntity.LinkedPipeline {
		return false
	}

	// compare Labels
	if len(currentEntity.Labels) != len(desiredEntity.Labels) {
		return false
	}
	for i, v := range currentEntity.Labels {
		if v != desiredEntity.Labels[i] {
			return false
		}
	}

	// compare Metadata
	if !reflect.DeepEqual(currentEntity.Metadata, desiredEntity.Metadata) {
		return false
	}

	return true
}
