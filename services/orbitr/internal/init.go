package orbitr

import (
	"context"
	"net/http"
	"runtime"

	"github.com/getsentry/raven-go"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/lantah/go/exp/orderbook"
	"github.com/lantah/go/services/orbitr/internal/db2/history"
	"github.com/lantah/go/services/orbitr/internal/ingest"
	"github.com/lantah/go/services/orbitr/internal/paths"
	"github.com/lantah/go/services/orbitr/internal/simplepath"
	"github.com/lantah/go/services/orbitr/internal/txsub"
	"github.com/lantah/go/services/orbitr/internal/txsub/sequence"
	"github.com/lantah/go/support/db"
	"github.com/lantah/go/support/log"
)

func mustNewDBSession(subservice db.Subservice, databaseURL string, maxIdle, maxOpen int, registry *prometheus.Registry, clientConfigs ...db.ClientConfig) db.SessionInterface {
	log.Infof("Establishing database session for %v", subservice)
	session, err := db.Open("postgres", databaseURL, clientConfigs...)
	if err != nil {
		log.Fatalf("cannot open %v DB: %v", subservice, err)
	}

	session.DB.SetMaxIdleConns(maxIdle)
	session.DB.SetMaxOpenConns(maxOpen)
	return db.RegisterMetrics(session, "orbitr", subservice, registry)
}

func mustInitOrbitRDB(app *App) {
	log.Infof("Initializing database...")

	maxIdle := app.config.OrbitRDBMaxIdleConnections
	maxOpen := app.config.OrbitRDBMaxOpenConnections
	if app.config.Ingest {
		maxIdle -= ingest.MaxDBConnections
		maxOpen -= ingest.MaxDBConnections
		if maxIdle <= 0 {
			log.Fatalf("max idle connections to orbitr db must be greater than %d", ingest.MaxDBConnections)
		}
		if maxOpen <= 0 {
			log.Fatalf("max open connections to orbitr db must be greater than %d", ingest.MaxDBConnections)
		}
	}

	if app.config.RoDatabaseURL == "" {
		var clientConfigs []db.ClientConfig
		if !app.config.Ingest {
			// if we are not ingesting then we don't expect to have long db queries / transactions
			clientConfigs = append(
				clientConfigs,
				db.StatementTimeout(app.config.ConnectionTimeout),
				db.IdleTransactionTimeout(app.config.ConnectionTimeout),
			)
		}
		app.historyQ = &history.Q{mustNewDBSession(
			db.HistorySubservice,
			app.config.DatabaseURL,
			maxIdle,
			maxOpen,
			app.prometheusRegistry,
			clientConfigs...,
		)}
	} else {
		// If RO set, use it for all DB queries
		roClientConfigs := []db.ClientConfig{
			db.StatementTimeout(app.config.ConnectionTimeout),
			db.IdleTransactionTimeout(app.config.ConnectionTimeout),
		}
		app.historyQ = &history.Q{mustNewDBSession(
			db.HistorySubservice,
			app.config.RoDatabaseURL,
			maxIdle,
			maxOpen,
			app.prometheusRegistry,
			roClientConfigs...,
		)}

		app.primaryHistoryQ = &history.Q{mustNewDBSession(
			db.HistoryPrimarySubservice,
			app.config.DatabaseURL,
			maxIdle,
			maxOpen,
			app.prometheusRegistry,
		)}
	}
}

func initIngester(app *App) {
	var err error
	var coreSession db.SessionInterface
	if !app.config.EnableCaptiveCoreIngestion {
		coreSession = mustNewDBSession(
			db.CoreSubservice, app.config.GravityDatabaseURL, ingest.MaxDBConnections, ingest.MaxDBConnections, app.prometheusRegistry)
	}
	app.ingester, err = ingest.NewSystem(ingest.Config{
		CoreSession: coreSession,
		HistorySession: mustNewDBSession(
			db.IngestSubservice, app.config.DatabaseURL, ingest.MaxDBConnections, ingest.MaxDBConnections, app.prometheusRegistry,
		),
		NetworkPassphrase:                    app.config.NetworkPassphrase,
		HistoryArchiveURLs:                   app.config.HistoryArchiveURLs,
		CheckpointFrequency:                  app.config.CheckpointFrequency,
		GravityURL:                       app.config.GravityURL,
		GravityCursor:                    app.config.CursorName,
		CaptiveCoreBinaryPath:                app.config.CaptiveCoreBinaryPath,
		CaptiveCoreStoragePath:               app.config.CaptiveCoreStoragePath,
		CaptiveCoreConfigUseDB:               app.config.CaptiveCoreConfigUseDB,
		CaptiveCoreToml:                      app.config.CaptiveCoreToml,
		RemoteCaptiveCoreURL:                 app.config.RemoteCaptiveCoreURL,
		EnableCaptiveCore:                    app.config.EnableCaptiveCoreIngestion,
		DisableStateVerification:             app.config.IngestDisableStateVerification,
		StateVerificationCheckpointFrequency: uint32(app.config.IngestStateVerificationCheckpointFrequency),
		StateVerificationTimeout:             app.config.IngestStateVerificationTimeout,
		EnableReapLookupTables:               app.config.HistoryRetentionCount > 0,
		EnableExtendedLogLedgerStats:         app.config.IngestEnableExtendedLogLedgerStats,
		RoundingSlippageFilter:               app.config.RoundingSlippageFilter,
		EnableIngestionFiltering:             app.config.EnableIngestionFiltering,
	})

	if err != nil {
		log.Fatal(err)
	}
}

func initPathFinder(app *App) {
	if app.config.DisablePathFinding {
		return
	}
	orderBookGraph := orderbook.NewOrderBookGraph()
	app.orderBookStream = ingest.NewOrderBookStream(
		&history.Q{app.OrbitRSession()},
		orderBookGraph,
	)

	var finder paths.Finder = simplepath.NewInMemoryFinder(orderBookGraph, !app.config.DisablePoolPathFinding)
	if app.config.MaxPathFindingRequests != 0 {
		finder = paths.NewRateLimitedFinder(finder, app.config.MaxPathFindingRequests)
	}
	app.paths = finder
}

// initSentry initialized the default sentry client with the configured DSN
func initSentry(app *App) {
	if app.config.SentryDSN == "" {
		return
	}

	log.WithField("dsn", app.config.SentryDSN).Info("Initializing sentry")
	err := raven.SetDSN(app.config.SentryDSN)
	if err != nil {
		log.Fatal(err)
	}
}

// initLogglyLog attaches a loggly hook to our logging system.
func initLogglyLog(app *App) {
	if app.config.LogglyToken == "" {
		return
	}

	log.WithFields(log.F{
		"token": app.config.LogglyToken,
		"tag":   app.config.LogglyTag,
	}).Info("Initializing loggly hook")

	hook := log.NewLogglyHook(app.config.LogglyToken, app.config.LogglyTag)
	log.DefaultLogger.AddHook(hook)

	go func() {
		<-app.ctx.Done()
		hook.Flush()
	}()
}

func initDbMetrics(app *App) {
	app.buildInfoGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Namespace: "orbitr", Subsystem: "build", Name: "info"},
		[]string{"version", "goversion"},
	)
	app.prometheusRegistry.MustRegister(app.buildInfoGauge)
	app.buildInfoGauge.With(prometheus.Labels{
		"version":   app.orbitrVersion,
		"goversion": runtime.Version(),
	}).Inc()

	app.ingestingGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{Namespace: "orbitr", Subsystem: "ingest", Name: "enabled"},
	)
	app.prometheusRegistry.MustRegister(app.ingestingGauge)

	app.ledgerState.RegisterMetrics(app.prometheusRegistry)

	app.coreState.RegisterMetrics(app.prometheusRegistry)

	if !app.config.DisablePathFinding {
		app.prometheusRegistry.MustRegister(app.orderBookStream.LatestLedgerGauge)
	}
}

// initGoMetrics registers the Go collector provided by prometheus package which
// includes Go-related metrics.
func initGoMetrics(app *App) {
	app.prometheusRegistry.MustRegister(prometheus.NewGoCollector())
}

// initProcessMetrics registers the process collector provided by prometheus
// package. This is only available on operating systems with a Linux-style proc
// filesystem and on Microsoft Windows.
func initProcessMetrics(app *App) {
	app.prometheusRegistry.MustRegister(
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
	)
}

// initIngestMetrics registers the metrics for the ingestion into the provided
// app's metrics registry.
func initIngestMetrics(app *App) {
	if app.ingester == nil {
		return
	}

	app.ingestingGauge.Inc()
	app.ingester.RegisterMetrics(app.prometheusRegistry)
}

func initTxSubMetrics(app *App) {
	app.submitter.Init()
	app.submitter.RegisterMetrics(app.prometheusRegistry)
}

func initWebMetrics(app *App) {
	app.webServer.RegisterMetrics(app.prometheusRegistry)
}

func initSubmissionSystem(app *App) {
	app.submitter = &txsub.System{
		Pending:         txsub.NewDefaultSubmissionList(),
		Submitter:       txsub.NewDefaultSubmitter(http.DefaultClient, app.config.GravityURL),
		SubmissionQueue: sequence.NewManager(),
		DB: func(ctx context.Context) txsub.OrbitRDB {
			return &history.Q{SessionInterface: app.OrbitRSession()}
		},
	}
}
