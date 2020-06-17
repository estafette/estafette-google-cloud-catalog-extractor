package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	contracts "github.com/estafette/estafette-ci-contracts"
	foundation "github.com/estafette/estafette-foundation"
	"github.com/opentracing-contrib/go-stdlib/nethttp"
	"github.com/opentracing/opentracing-go"
	"github.com/rs/zerolog/log"
	"github.com/sethgrid/pester"
)

type ApiClient interface {
	GetToken(ctx context.Context) (token string, err error)
	GetCatalogEntities(ctx context.Context, parentKey, parentValue, entityKey string) (entities []*contracts.CatalogEntity, err error)
	CreateCatalogEntity(ctx context.Context, entity *contracts.CatalogEntity) (err error)
	UpdateCatalogEntity(ctx context.Context, entity *contracts.CatalogEntity) (err error)
	DeleteCatalogEntity(ctx context.Context, entity *contracts.CatalogEntity) (err error)
}

// NewApiClient returns a new ApiClient
func NewApiClient(apiBaseURL, clientID, clientSecret string) ApiClient {
	return &apiClient{
		apiBaseURL:   apiBaseURL,
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

type apiClient struct {
	apiBaseURL   string
	clientID     string
	clientSecret string
	token        string
}

func (c *apiClient) GetToken(ctx context.Context) (token string, err error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "ApiClient::GetToken")
	defer span.Finish()

	clientObject := contracts.Client{
		ClientID:     c.clientID,
		ClientSecret: c.clientSecret,
	}

	bytes, err := json.Marshal(clientObject)
	if err != nil {
		return
	}

	getTokenURL := fmt.Sprintf("%v/api/auth/client/login", c.apiBaseURL)
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	responseBody, err := c.postRequest(getTokenURL, span, strings.NewReader(string(bytes)), headers)

	tokenResponse := struct {
		Token string `json:"token"`
	}{}

	// unmarshal json body
	err = json.Unmarshal(responseBody, &tokenResponse)
	if err != nil {
		log.Error().Err(err).Str("body", string(responseBody)).Msgf("Failed unmarshalling get token response")
		return
	}

	// set token
	c.token = tokenResponse.Token

	return tokenResponse.Token, nil
}

func (c *apiClient) GetCatalogEntities(ctx context.Context, parentKey, parentValue, entityKey string) (entities []*contracts.CatalogEntity, err error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "ApiClient::GetCatalogEntities")
	defer span.Finish()

	pageNumber := 1
	pageSize := 100
	entities = make([]*contracts.CatalogEntity, 0)

	for {
		ents, pagination, err := c.getCatalogEntitiesPage(ctx, parentKey, parentValue, entityKey, pageNumber, pageSize)
		if err != nil {
			return entities, err
		}
		entities = append(entities, ents...)

		if pagination.TotalPages <= pageNumber {
			break
		}

		pageNumber++
	}

	span.LogKV("entities", len(entities))

	return entities, nil
}

func (c *apiClient) getCatalogEntitiesPage(ctx context.Context, parentKey, parentValue, entityKey string, pageNumber, pageSize int) (entities []*contracts.CatalogEntity, pagination contracts.Pagination, err error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "ApiClient::getCatalogEntitiesPage")
	defer span.Finish()

	span.LogKV("page[number]", pageNumber, "page[size]", pageSize)

	parentFilter := ""
	if parentKey != "" {
		parentFilter = parentKey
		if parentValue != "" {
			parentFilter = "=" + parentValue
		}

		parentFilter = "&filter[parent]=" + url.QueryEscape(parentFilter)
	}

	entityFilter := ""
	if entityKey != "" {
		entityFilter = "&filter[entity]=" + url.QueryEscape(entityKey)
	}

	getCatalogEntitiesURL := fmt.Sprintf("%v/api/catalog/entities?page[number]=%v&page[size]=%v%v%v", c.apiBaseURL, pageNumber, pageSize, parentFilter, entityFilter)
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %v", c.token),
		"Content-Type":  "application/json",
	}

	responseBody, err := c.getRequest(getCatalogEntitiesURL, span, nil, headers)

	var listResponse struct {
		Items      []*contracts.CatalogEntity `json:"items"`
		Pagination contracts.Pagination       `json:"pagination"`
	}

	// unmarshal json body
	err = json.Unmarshal(responseBody, &listResponse)
	if err != nil {
		log.Error().Err(err).Str("body", string(responseBody)).Str("url", getCatalogEntitiesURL).Msgf("Failed unmarshalling get catalog entities response")
		return
	}

	entities = listResponse.Items

	span.LogKV("entities", len(entities))

	return entities, listResponse.Pagination, nil
}

func (c *apiClient) CreateCatalogEntity(ctx context.Context, entity *contracts.CatalogEntity) (err error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "ApiClient::CreateCatalogEntity")
	defer span.Finish()

	span.LogKV("parent", fmt.Sprintf("%v=%v", entity.ParentKey, entity.ParentValue))
	span.LogKV("entity", fmt.Sprintf("%v=%v", entity.Key, entity.Value))

	c.AddLabelIfMissing(ctx, entity)

	bytes, err := json.Marshal(entity)
	if err != nil {
		return
	}

	createCatalogEntityURL := fmt.Sprintf("%v/api/catalog/entities", c.apiBaseURL)
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %v", c.token),
		"Content-Type":  "application/json",
	}

	_, err = c.postRequest(createCatalogEntityURL, span, strings.NewReader(string(bytes)), headers, http.StatusCreated)

	return
}

func (c *apiClient) UpdateCatalogEntity(ctx context.Context, entity *contracts.CatalogEntity) (err error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "ApiClient::UpdateCatalogEntity")
	defer span.Finish()

	span.LogKV("parent", fmt.Sprintf("%v=%v", entity.ParentKey, entity.ParentValue))
	span.LogKV("entity", fmt.Sprintf("%v=%v", entity.Key, entity.Value))

	c.AddLabelIfMissing(ctx, entity)

	bytes, err := json.Marshal(entity)
	if err != nil {
		return
	}

	updateCatalogEntityURL := fmt.Sprintf("%v/api/catalog/entities/%v", c.apiBaseURL, entity.ID)
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %v", c.token),
		"Content-Type":  "application/json",
	}

	_, err = c.putRequest(updateCatalogEntityURL, span, strings.NewReader(string(bytes)), headers)

	return
}

func (c *apiClient) DeleteCatalogEntity(ctx context.Context, entity *contracts.CatalogEntity) (err error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "ApiClient::DeleteCatalogEntity")
	defer span.Finish()

	span.LogKV("parent", fmt.Sprintf("%v=%v", entity.ParentKey, entity.ParentValue))
	span.LogKV("entity", fmt.Sprintf("%v=%v", entity.Key, entity.Value))

	bytes, err := json.Marshal(entity)
	if err != nil {
		return
	}

	deleteCatalogEntityURL := fmt.Sprintf("%v/api/catalog/entities/%v", c.apiBaseURL, entity.ID)
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %v", c.token),
		"Content-Type":  "application/json",
	}

	_, err = c.deleteRequest(deleteCatalogEntityURL, span, strings.NewReader(string(bytes)), headers)

	return
}

func (c *apiClient) AddLabelIfMissing(ctx context.Context, entity *contracts.CatalogEntity) {
	if entity != nil {
		hasOwnKeyValueAsLabel := false
		for _, l := range entity.Labels {
			if l.Key == entity.Key && l.Value == entity.Value {
				hasOwnKeyValueAsLabel = true
			}
		}
		if !hasOwnKeyValueAsLabel {
			entity.Labels = append(entity.Labels, contracts.Label{
				Key:   entity.Key,
				Value: entity.Value,
			})
		}
	}
}

func (c *apiClient) getRequest(uri string, span opentracing.Span, requestBody io.Reader, headers map[string]string, allowedStatusCodes ...int) (responseBody []byte, err error) {
	return c.makeRequest("GET", uri, span, requestBody, headers, allowedStatusCodes...)
}

func (c *apiClient) postRequest(uri string, span opentracing.Span, requestBody io.Reader, headers map[string]string, allowedStatusCodes ...int) (responseBody []byte, err error) {
	return c.makeRequest("POST", uri, span, requestBody, headers, allowedStatusCodes...)
}

func (c *apiClient) putRequest(uri string, span opentracing.Span, requestBody io.Reader, headers map[string]string, allowedStatusCodes ...int) (responseBody []byte, err error) {
	return c.makeRequest("PUT", uri, span, requestBody, headers, allowedStatusCodes...)
}

func (c *apiClient) deleteRequest(uri string, span opentracing.Span, requestBody io.Reader, headers map[string]string, allowedStatusCodes ...int) (responseBody []byte, err error) {
	return c.makeRequest("DELETE", uri, span, requestBody, headers, allowedStatusCodes...)
}

func (c *apiClient) makeRequest(method, uri string, span opentracing.Span, requestBody io.Reader, headers map[string]string, allowedStatusCodes ...int) (responseBody []byte, err error) {

	// create client, in order to add headers
	client := pester.NewExtendedClient(&http.Client{Transport: &nethttp.Transport{}})
	client.MaxRetries = 3
	client.Backoff = pester.ExponentialJitterBackoff
	client.KeepLog = true
	client.Timeout = time.Second * 10

	request, err := http.NewRequest(method, uri, requestBody)
	if err != nil {
		return nil, err
	}

	// add tracing context
	request = request.WithContext(opentracing.ContextWithSpan(request.Context(), span))

	// collect additional information on setting up connections
	request, ht := nethttp.TraceRequest(span.Tracer(), request)

	// add headers
	for k, v := range headers {
		request.Header.Add(k, v)
	}

	// perform actual request
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	ht.Finish()

	if len(allowedStatusCodes) == 0 {
		allowedStatusCodes = []int{http.StatusOK}
	}

	if !foundation.IntArrayContains(allowedStatusCodes, response.StatusCode) {
		return nil, fmt.Errorf("%v responded with status code %v", uri, response.StatusCode)
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return
	}

	return body, nil
}
