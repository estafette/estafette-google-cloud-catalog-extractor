package main

import (
	"context"
	"io"
	"runtime"

	"github.com/alecthomas/kingpin"
	foundation "github.com/estafette/estafette-foundation"
	"github.com/opentracing/opentracing-go"
	"github.com/rs/zerolog/log"
	"github.com/uber/jaeger-client-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
)

var (
	appgroup  string
	app       string
	version   string
	branch    string
	revision  string
	buildDate string
	goVersion = runtime.Version()

	// params for apiClient
	apiBaseURL   = kingpin.Flag("api-base-url", "The base url of the estafette-ci-api to communicate with").Envar("API_BASE_URL").Required().String()
	clientID     = kingpin.Flag("client-id", "The id of the client as configured in Estafette, to securely communicate with the api.").Envar("CLIENT_ID").Required().String()
	clientSecret = kingpin.Flag("client-secret", "The secret of the client as configured in Estafette, to securely communicate with the api.").Envar("CLIENT_SECRET").Required().String()

	// params for gsuiteClient
	organization = kingpin.Flag("organization", "The name of the Estafette organization these cloud assets belong to.").Envar("ORGANIZATION").Required().String()
)

const (
	organizationKeyName    = "organization"
	cloudKeyName           = "cloud"
	projectKeyName         = "project"
	gkeClusterKeyName      = "gke-cluster"
	pubsubTopicKeyName     = "pubsub-topic"
	cloudfunctionKeyName   = "cloud-function"
	storageBucketKeyName   = "storage-bucket"
	dataflowJobKeyName     = "dataflow-job"
	bigqueryDatasetKeyName = "bigquery-dataset"
	bigqueryTableKeyName   = "bigquery-table"
)

const cloudKeyValue = "Google Cloud"

const locationLabelKey = "location"

func main() {

	// parse command line parameters
	kingpin.Parse()

	// init log format from envvar ESTAFETTE_LOG_FORMAT
	foundation.InitLoggingFromEnv(foundation.NewApplicationInfo(appgroup, app, version, branch, revision, buildDate))

	closer := initJaeger(app)
	defer closer.Close()

	ctx := context.Background()

	span, ctx := opentracing.StartSpanFromContext(ctx, "Main")
	defer span.Finish()

	apiClient := NewApiClient(*apiBaseURL, *clientID, *clientSecret)
	googleCloudClient, err := NewGoogleCloudClient(ctx)
	handleError(closer, err, "Failed creating GoogleCloudClient")

	extractor := NewExtractor(apiClient, googleCloudClient)
	err = extractor.Run(ctx, *organization)
	handleError(closer, err, "Failed running extraction")

	log.Info().Msg("Done!")
}

func handleError(jaegerCloser io.Closer, err error, message string) {
	if err != nil {
		jaegerCloser.Close()
		log.Fatal().Err(err).Msg(message)
	}
}

// initJaeger returns an instance of Jaeger Tracer that can be configured with environment variables
// https://github.com/jaegertracing/jaeger-client-go#environment-variables
func initJaeger(service string) io.Closer {

	cfg, err := jaegercfg.FromEnv()
	if err != nil {
		log.Fatal().Err(err).Msg("Generating Jaeger config from environment variables failed")
	}

	closer, err := cfg.InitGlobalTracer(service, jaegercfg.Logger(jaeger.StdLogger))
	if err != nil {
		log.Fatal().Err(err).Msg("Generating Jaeger tracer failed")
	}

	return closer
}
