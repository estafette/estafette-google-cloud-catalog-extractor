package main

import (
	"testing"

	contracts "github.com/estafette/estafette-ci-contracts"
	"github.com/stretchr/testify/assert"
)

func TestEntitiesAreEqual(t *testing.T) {
	t.Run("ReturnsTrueIfEntitiesAreTheSame", func(t *testing.T) {

		extractor := &extractor{}
		currentCatalogEntity := &contracts.CatalogEntity{}
		desiredCatalogEntity := currentCatalogEntity

		// act
		equal := extractor.entitiesAreEqual(currentCatalogEntity, desiredCatalogEntity)

		assert.True(t, equal)
	})

	t.Run("ReturnsTrueIfLinkedPipelineDoesMatch", func(t *testing.T) {

		extractor := &extractor{}
		currentCatalogEntity := &contracts.CatalogEntity{
			LinkedPipeline: "github.com/estafette/estafette-ci-api",
		}
		desiredCatalogEntity := &contracts.CatalogEntity{
			LinkedPipeline: "github.com/estafette/estafette-ci-api",
		}

		// act
		equal := extractor.entitiesAreEqual(currentCatalogEntity, desiredCatalogEntity)

		assert.True(t, equal)
	})

	t.Run("ReturnsFalseIfLinkedPipelineDoesNotMatch", func(t *testing.T) {

		extractor := &extractor{}
		currentCatalogEntity := &contracts.CatalogEntity{
			LinkedPipeline: "github.com/estafette/estafette-ci-api",
		}
		desiredCatalogEntity := &contracts.CatalogEntity{
			LinkedPipeline: "github.com/estafette/estafette-ci-builder",
		}

		// act
		equal := extractor.entitiesAreEqual(currentCatalogEntity, desiredCatalogEntity)

		assert.False(t, equal)
	})

	t.Run("ReturnsTrueIfLabelsDoMatch", func(t *testing.T) {

		extractor := &extractor{}
		currentCatalogEntity := &contracts.CatalogEntity{
			Labels: []contracts.Label{
				{
					Key:   "organization",
					Value: "Estafette",
				},
				{
					Key:   "cloud",
					Value: "Google Cloud",
				},
			},
		}
		desiredCatalogEntity := &contracts.CatalogEntity{
			Labels: []contracts.Label{
				{
					Key:   "organization",
					Value: "Estafette",
				},
				{
					Key:   "cloud",
					Value: "Google Cloud",
				},
			},
		}

		// act
		equal := extractor.entitiesAreEqual(currentCatalogEntity, desiredCatalogEntity)

		assert.True(t, equal)
	})

	t.Run("ReturnsFalseIfLabelsDoNotMatch", func(t *testing.T) {

		extractor := &extractor{}
		currentCatalogEntity := &contracts.CatalogEntity{
			Labels: []contracts.Label{
				{
					Key:   "organization",
					Value: "Estafette",
				},
				{
					Key:   "cloud",
					Value: "Google Cloud",
				},
			},
		}
		desiredCatalogEntity := &contracts.CatalogEntity{
			Labels: []contracts.Label{
				{
					Key:   "organization",
					Value: "Estafette",
				},
				{
					Key:   "cloud",
					Value: "AWS",
				},
			},
		}

		// act
		equal := extractor.entitiesAreEqual(currentCatalogEntity, desiredCatalogEntity)

		assert.False(t, equal)
	})

	t.Run("ReturnsTrueIfMetadataDoesMatch", func(t *testing.T) {

		extractor := &extractor{}
		currentCatalogEntity := &contracts.CatalogEntity{
			Metadata: map[string]interface{}{
				"href": "/project/myproject",
			},
		}
		desiredCatalogEntity := &contracts.CatalogEntity{
			Metadata: map[string]interface{}{
				"href": "/project/myproject",
			},
		}

		// act
		equal := extractor.entitiesAreEqual(currentCatalogEntity, desiredCatalogEntity)

		assert.True(t, equal)
	})

	t.Run("ReturnsFalseIfMetadataDoesNotMatch", func(t *testing.T) {

		extractor := &extractor{}
		currentCatalogEntity := &contracts.CatalogEntity{
			Metadata: map[string]interface{}{
				"href": "/project/myproject",
			},
		}
		desiredCatalogEntity := &contracts.CatalogEntity{
			Metadata: map[string]interface{}{
				"href": "/project/yourproject",
			},
		}

		// act
		equal := extractor.entitiesAreEqual(currentCatalogEntity, desiredCatalogEntity)

		assert.False(t, equal)
	})
}
