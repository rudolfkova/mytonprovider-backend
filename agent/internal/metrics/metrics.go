package metrics

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	providerchecksv1 "mytonprovider-contracts/gen/go/providerchecks/v1"
)

var (
	grpcRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "agent_grpc_request_duration_seconds",
			Help:    "gRPC unary request wall time in seconds",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 15, 30, 60, 120},
		},
		[]string{"grpc_method", "grpc_code"},
	)
	grpcRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agent_grpc_requests_total",
			Help: "gRPC unary requests completed",
		},
		[]string{"grpc_method", "grpc_code"},
	)
	runChecksContractResults = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agent_runchecks_contract_results_total",
			Help: "RunChecks contract-level results by reason_code",
		},
		[]string{"reason"},
	)
	runStorageRatesRows = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agent_runstoragerates_rows_total",
			Help: "RunStorageRates per-row outcomes",
		},
		[]string{"ok"},
	)
	runChecksJobsCompleted = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "agent_runchecks_jobs_completed_total",
			Help: "RunChecks unary RPC completed with a response (one increment per finished handler)",
		},
	)
	runChecksJobDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "agent_runchecks_job_duration_seconds",
			Help:    "Wall time of one RunChecks RPC (handler), excluding serialization to client",
			Buckets: []float64{.5, 1, 2, 5, 10, 30, 60, 120, 300, 600, 1200, 3600},
		},
	)
	runChecksJobContractResults = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "agent_runchecks_job_contract_results",
			Help:    "How many contract results one RunChecks RPC returned (len(Results))",
			Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 20000},
		},
	)
	runStorageRatesJobsCompleted = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "agent_runstoragerates_jobs_completed_total",
			Help: "RunStorageRates RPCs finished (gRPC OK), one increment per response",
		},
	)
	runStorageRatesJobDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "agent_runstoragerates_job_duration_seconds",
			Help:    "Wall time of one RunStorageRates RPC (handler)",
			Buckets: []float64{.05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60, 120, 300},
		},
	)
)

func init() {
	methods := []string{
		grpc_health_v1.Health_Check_FullMethodName,
		providerchecksv1.ProviderChecksService_RunChecks_FullMethodName,
		providerchecksv1.ProviderChecksService_RunStorageRates_FullMethodName,
	}
	codeNames := []string{
		codes.OK.String(),
		codes.Unknown.String(),
		codes.InvalidArgument.String(),
		codes.Unauthenticated.String(),
		codes.DeadlineExceeded.String(),
		codes.Unavailable.String(),
		codes.Internal.String(),
		codes.ResourceExhausted.String(),
		codes.NotFound.String(),
	}
	for _, m := range methods {
		for _, c := range codeNames {
			grpcRequestsTotal.WithLabelValues(m, c).Add(0)
		}
		grpcRequestDuration.WithLabelValues(m, codes.OK.String()).Observe(0)
	}
	for _, reasonLabel := range providerchecksv1.ReasonCode_name {
		runChecksContractResults.WithLabelValues(reasonLabel).Add(0)
	}
	runStorageRatesRows.WithLabelValues("true").Add(0)
	runStorageRatesRows.WithLabelValues("false").Add(0)
}

// UnaryServerInterceptor records request count and duration (outer interceptor: includes auth failures).
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		code := codeFromErr(err)
		method := info.FullMethod
		l := prometheus.Labels{"grpc_method": method, "grpc_code": code.String()}
		grpcRequestsTotal.With(l).Inc()
		grpcRequestDuration.With(l).Observe(time.Since(start).Seconds())
		return resp, err
	}
}

func codeFromErr(err error) codes.Code {
	if err == nil {
		return codes.OK
	}
	if s, ok := status.FromError(err); ok {
		return s.Code()
	}
	return codes.Unknown
}

// IncRunChecksResult increments per-contract reason (bounded enum string).
func IncRunChecksResult(reason string) {
	runChecksContractResults.WithLabelValues(reason).Inc()
}

// IncRunStorageRatesRow increments per-row ok/fail.
func IncRunStorageRatesRow(ok bool) {
	s := "false"
	if ok {
		s = "true"
	}
	runStorageRatesRows.WithLabelValues(s).Inc()
}

// ObserveRunChecksJob records one finished RunChecks RPC (call after per-contract counters are updated).
// Intentionally no job_id label — unbounded cardinality would break Prometheus.
func ObserveRunChecksJob(duration time.Duration, contractResults int) {
	runChecksJobsCompleted.Inc()
	runChecksJobDuration.Observe(duration.Seconds())
	if contractResults > 0 {
		runChecksJobContractResults.Observe(float64(contractResults))
	}
}

// ObserveRunStorageRatesJob records one finished RunStorageRates RPC.
func ObserveRunStorageRatesJob(duration time.Duration) {
	runStorageRatesJobsCompleted.Inc()
	runStorageRatesJobDuration.Observe(duration.Seconds())
}
