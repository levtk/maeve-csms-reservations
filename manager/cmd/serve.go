// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/subnova/slog-exporter/slogtrace"
	"github.com/thoughtworks/maeve-csms/manager/mqtt"
	"github.com/thoughtworks/maeve-csms/manager/server"
	"github.com/thoughtworks/maeve-csms/manager/services"
	"github.com/thoughtworks/maeve-csms/manager/store"
	"github.com/thoughtworks/maeve-csms/manager/store/firestore"
	"github.com/thoughtworks/maeve-csms/manager/store/inmemory"
	"go.mozilla.org/pkcs7"
	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"golang.org/x/exp/slog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

var (
	mqttAddr                  string
	mqttPrefix                string
	mqttGroup                 string
	apiAddr                   string
	moTrustAnchorCertPEMFiles []string
	csoOPCPToken              string
	csoOPCPUrl                string
	moOPCPToken               string
	moOPCPUrl                 string
	moRootCertPool            string
	storageEngine             string
	gcloudProject             string
	keyLogFile                string
	otelCollectorAddr         string
	logFormat                 string
)

// Initializes an OTLP exporter, and configures the corresponding trace and
// metric providers.
func initProvider(collectorAddr string) (func(context.Context) error, error) {
	ctx := context.Background()

	var err error
	var res *resource.Resource
	var traceExporter trace.SpanExporter

	if collectorAddr != "" {
		res, err = resource.New(ctx,
			resource.WithDetectors(gcp.NewDetector()),
			resource.WithTelemetrySDK(),
			resource.WithAttributes(
				// the service name used to display traces in backends
				semconv.ServiceName("maeve-csms-manager"),
			),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create resource: %w", err)
		}

		ctx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		conn, err := grpc.DialContext(ctx, collectorAddr,
			// Note the use of insecure transport here. TLS is recommended in production.
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create gRPC connection to collector: %w", err)
		}

		// Set up a trace exporter
		traceExporter, err = otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
		if err != nil {
			return nil, fmt.Errorf("failed to create trace exporter: %w", err)
		}
	} else {
		res, err = resource.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to create resource: %w", err)
		}

		traceExporter, err = slogtrace.New()
		if err != nil {
			return nil, err
		}
	}

	// Register the trace exporter with a TracerProvider, using a batch
	// span processor to aggregate spans before export.
	bsp := trace.NewBatchSpanProcessor(traceExporter)
	tracerProvider := trace.NewTracerProvider(
		trace.WithSampler(trace.AlwaysSample()),
		trace.WithResource(res),
		trace.WithSpanProcessor(bsp),
	)
	otel.SetTracerProvider(tracerProvider)

	// set global propagator to tracecontext (the default is no-op).
	otel.SetTextMapPropagator(propagation.TraceContext{})

	// Shutdown will flush any remaining spans and shut down the exporter.
	return tracerProvider.Shutdown, nil
}

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the server",
	Long: `Starts the server which will subscribe to messages from
the gateway and send appropriate responses.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if logFormat == "json" {
			slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
		}

		shutdown, err := initProvider(otelCollectorAddr)
		if err != nil {
			return err
		}
		defer func() {
			err := shutdown(context.Background())
			if err != nil {
				slog.Error("shutting down OTLP exporter", "error", err)
			}
		}()

		tracer := otel.Tracer("manager")

		brokerUrl, err := url.Parse(mqttAddr)
		if err != nil {
			return fmt.Errorf("parsing mqtt broker url: %v", err)
		}

		var engine store.Engine
		switch storageEngine {
		case "firestore":
			engine, err = firestore.NewStore(context.Background(), gcloudProject)
			if err != nil {
				return err
			}
		case "inmemory":
			engine = inmemory.NewStore()
		default:
			return fmt.Errorf("unsupported storage engine %s", storageEngine)
		}

		apiServer := server.New("api", apiAddr, nil, server.NewApiHandler(engine))
		v2gCertificates, err := pullMoTrustAnchors(moRootCertPool)
		if err != nil {
			return fmt.Errorf("pulling certificates from mo root cert pool: %v", err)
		}

		tariffService := services.BasicKwhTariffService{}
		certValidationService := services.OnlineCertificateValidationService{
			RootCertificates: v2gCertificates,
			MaxOCSPAttempts:  3,
		}

		var transport http.RoundTripper

		if keyLogFile != "" {
			slog.Warn("***** TLS key logging enabled *****")

			//#nosec G304 - only files specified by the person running the application will be used
			keyLog, err := os.OpenFile(keyLogFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
			if err != nil {
				return fmt.Errorf("opening key log file: %v", err)
			}

			baseTransport := http.DefaultTransport.(*http.Transport).Clone()
			baseTransport.TLSClientConfig = &tls.Config{
				KeyLogWriter: keyLog,
				MinVersion:   tls.VersionTLS12,
			}

			transport = otelhttp.NewTransport(baseTransport)
		} else {
			transport = otelhttp.NewTransport(http.DefaultTransport)
		}

		httpClient := &http.Client{Transport: transport}

		var certSignerService services.CertificateSignerService
		var certProviderService services.EvCertificateProvider
		if csoOPCPToken != "" && csoOPCPUrl != "" {
			certSignerService = services.OpcpCpoCertificateSignerService{
				BaseURL:     csoOPCPUrl,
				BearerToken: csoOPCPToken,
				ISOVersion:  services.ISO15118V2,
				HttpClient:  httpClient,
			}
		}
		if moOPCPToken != "" && moOPCPUrl != "" {
			certProviderService = services.OpcpMoEvCertificateProvider{
				BaseURL:     moOPCPUrl,
				BearerToken: moOPCPToken,
				HttpClient:  httpClient,
				Tracer:      tracer,
			}
		}

		mqttHandler := mqtt.NewHandler(
			mqtt.WithMqttBrokerUrl(brokerUrl),
			mqtt.WithMqttPrefix(mqttPrefix),
			mqtt.WithMqttGroup(mqttGroup),
			mqtt.WithTariffService(tariffService),
			mqtt.WithCertValidationService(certValidationService),
			mqtt.WithCertSignerService(certSignerService),
			mqtt.WithCertificateProviderService(certProviderService),
			mqtt.WithStorageEngine(engine),
			mqtt.WithOtelTracer(tracer),
		)

		errCh := make(chan error, 1)
		apiServer.Start(errCh)
		mqttHandler.Connect(errCh)

		err = <-errCh
		return err
	},
}

func pullMoTrustAnchors(url string) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	body, err := retrieveCertificates(url)
	if err != nil {
		return nil, err
	}
	decodedBody, err := base64.StdEncoding.DecodeString(string(body))
	if err != nil {
		return nil, err
	}

	crtChain, err := pkcs7.Parse(decodedBody)
	if err != nil {
		return nil, err
	}

	for _, cert := range crtChain.Certificates {
		if cert.Issuer.String() == cert.Subject.String() {
			certs = append(certs, cert)
		}
	}
	return certs, nil
}

func retrieveCertificates(url string) ([]byte, error) {
	client := http.DefaultClient
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Accept", "application/pkcs10, application/pkcs7")
	req.Header.Add("Content-Transfer-Encoding", "application/pkcs10")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", moOPCPToken))

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	} else if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received code %v in response", resp.StatusCode)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func readCertificatesFromPEMFile(pemFile string) ([]*x509.Certificate, error) {
	//#nosec G304 - only files specified by the person running the application will be loaded
	pemData, err := os.ReadFile(pemFile)
	if err != nil {
		return nil, err
	}
	return parseCertificates(pemData)
}

func parseCertificates(pemData []byte) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	for {
		cert, rest, err := parseCertificate(pemData)
		if err != nil {
			return nil, err
		}
		if cert == nil {
			break
		}
		certs = append(certs, cert)
		pemData = rest
	}
	return certs, nil
}

func parseCertificate(pemData []byte) (cert *x509.Certificate, rest []byte, err error) {
	block, rest := pem.Decode(pemData)
	if block == nil {
		return
	}
	if block.Type != "CERTIFICATE" {
		return
	}
	cert, err = x509.ParseCertificate(block.Bytes)
	if err != nil {
		cert = nil
		return
	}
	return
}

func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.Flags().StringVarP(&mqttAddr, "mqtt-addr", "m", "mqtt://127.0.0.1:1883",
		"The address of the MQTT broker, e.g. mqtt://127.0.0.1:1883")
	serveCmd.Flags().StringVar(&mqttPrefix, "mqtt-prefix", "cs",
		"The MQTT topic prefix that the manager will subscribe to, e.g. cs")
	serveCmd.Flags().StringVar(&mqttGroup, "mqtt-group", "manager",
		"The MQTT group to use for the shared subscription, e.g. manager")
	serveCmd.Flags().StringVarP(&apiAddr, "api-addr", "a", "127.0.0.1:9410",
		"The address that the API server will listen on for connections, e.g. 127.0.0.1:9410")
	serveCmd.Flags().StringVar(&csoOPCPToken, "cso-opcp-token", "",
		"The token to use when integrating with the CSO OPCP (e.g. Hubject's token)")
	serveCmd.Flags().StringVar(&csoOPCPUrl, "cso-opcp-url", "https://open.plugncharge-test.hubject.com",
		"The Environment URL to integrate with the CSO OPCP (e.g. Hubject's environment)")
	serveCmd.Flags().StringVar(&moOPCPToken, "mo-opcp-token", "",
		"The token to use when integrating with the MO OPCP (e.g. Hubject's token)")
	serveCmd.Flags().StringVar(&moOPCPUrl, "mo-opcp-url", "https://open.plugncharge-test.hubject.com",
		"The Environment URL to integrate with the MO OPCP (e.g. Hubject's environment)")
	serveCmd.Flags().StringVarP(&storageEngine, "storage-engine", "s", "firestore",
		"The storage engine to use for persistence, one of [firestore, inmemory]")
	serveCmd.Flags().StringVar(&gcloudProject, "gcloud-project", "*detect-project-id*",
		"The google cloud project that hosts the firestore instance (if chosen storage-engine)")
	serveCmd.Flags().StringVar(&keyLogFile, "key-log-file", "",
		"File to write TLS key material to in NSS key log format (for debugging)")
	serveCmd.Flags().StringVar(&otelCollectorAddr, "otel-collector-addr", "",
		"The address of the open telemetry collector that will receive traces, e.g. localhost:4317")
	serveCmd.Flags().StringVar(&logFormat, "log-format", "text",
		"The format of the logs, one of [text, json]")
	serveCmd.Flags().StringVar(&moRootCertPool, "mo-root-certificate-pool", "https://open.plugncharge-test.hubject.com/mo/cacerts/ISO15118-2",
		"The address to retrieve the certificate pool containing the trust anchor for MO")
}
