/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package options

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/samber/lo"
	cliflag "k8s.io/component-base/cli/flag"

	"sigs.k8s.io/karpenter/pkg/utils/env"
)

var (
	validLogLevels = []string{"", "debug", "info", "error"}

	Injectables = []Injectable{&Options{}}
)

type optionsKey struct{}

type FeatureGates struct {
	Drift    bool
	inputStr string
}

// Options contains all CLI flags / env vars for karpenter-core. It adheres to the options.Injectable interface.
type Options struct {
	ServiceName          string
	DisableWebhook       bool
	WebhookPort          int
	MetricsPort          int
	WebhookMetricsPort   int
	HealthProbePort      int
	KubeClientQPS        int
	KubeClientBurst      int
	EnableProfiling      bool
	EnableLeaderElection bool
	MemoryLimit          int64
	LogLevel             string
	BatchMaxDuration     time.Duration
	BatchIdleDuration    time.Duration
	FeatureGates         FeatureGates
}

type FlagSet struct {
	*flag.FlagSet
}

// BoolVarWithEnv defines a bool flag with a specified name, default value, usage string, and fallback environment
// variable.
func (fs *FlagSet) BoolVarWithEnv(p *bool, name string, envVar string, val bool, usage string) {
	*p = env.WithDefaultBool(envVar, val)
	fs.BoolFunc(name, usage, func(val string) error {
		if val != "true" && val != "false" {
			return fmt.Errorf("%q is not a valid value, must be true or false", val)
		}
		*p = (val) == "true"
		return nil
	})
}

func (o *Options) AddFlags(fs *FlagSet) {
	fs.StringVar(&o.ServiceName, "karpenter-service", env.WithDefaultString("KARPENTER_SERVICE", ""), "The Karpenter Service name for the dynamic webhook certificate")
	fs.BoolVarWithEnv(&o.DisableWebhook, "disable-webhook", "DISABLE_WEBHOOK", true, "Disable the admission and validation webhooks")
	fs.IntVar(&o.WebhookPort, "webhook-port", env.WithDefaultInt("WEBHOOK_PORT", 8443), "The port the webhook endpoint binds to for validation and mutation of resources")
	fs.IntVar(&o.MetricsPort, "metrics-port", env.WithDefaultInt("METRICS_PORT", 8000), "The port the metric endpoint binds to for operating metrics about the controller itself")
	fs.IntVar(&o.WebhookMetricsPort, "webhook-metrics-port", env.WithDefaultInt("WEBHOOK_METRICS_PORT", 8001), "The port the webhook metric endpoing binds to for operating metrics about the webhook")
	fs.IntVar(&o.HealthProbePort, "health-probe-port", env.WithDefaultInt("HEALTH_PROBE_PORT", 8081), "The port the health probe endpoint binds to for reporting controller health")
	fs.IntVar(&o.KubeClientQPS, "kube-client-qps", env.WithDefaultInt("KUBE_CLIENT_QPS", 200), "The smoothed rate of qps to kube-apiserver")
	fs.IntVar(&o.KubeClientBurst, "kube-client-burst", env.WithDefaultInt("KUBE_CLIENT_BURST", 300), "The maximum allowed burst of queries to the kube-apiserver")
	fs.BoolVarWithEnv(&o.EnableProfiling, "enable-profiling", "ENABLE_PROFILING", false, "Enable the profiling on the metric endpoint")
	fs.BoolVarWithEnv(&o.EnableLeaderElection, "leader-elect", "LEADER_ELECT", true, "Start leader election client and gain leadership before executing the main loop. Enable this when running replicated components for high availability.")
	fs.Int64Var(&o.MemoryLimit, "memory-limit", env.WithDefaultInt64("MEMORY_LIMIT", -1), "Memory limit on the container running the controller. The GC soft memory limit is set to 90% of this value.")
	fs.StringVar(&o.LogLevel, "log-level", env.WithDefaultString("LOG_LEVEL", "info"), "Log verbosity level. Can be one of 'debug', 'info', or 'error'")
	fs.DurationVar(&o.BatchMaxDuration, "batch-max-duration", env.WithDefaultDuration("BATCH_MAX_DURATION", 10*time.Second), "The maximum length of a batch window. The longer this is, the more pods we can consider for provisioning at one time which usually results in fewer but larger nodes.")
	fs.DurationVar(&o.BatchIdleDuration, "batch-idle-duration", env.WithDefaultDuration("BATCH_IDLE_DURATION", time.Second), "The maximum amount of time with no new pending pods that if exceeded ends the current batching window. If pods arrive faster than this time, the batching window will be extended up to the maxDuration. If they arrive slower, the pods will be batched separately.")
	fs.StringVar(&o.FeatureGates.inputStr, "feature-gates", env.WithDefaultString("FEATURE_GATES", "Drift=true"), "Optional features can be enabled / disabled using feature gates. Current options are: Drift")
}

func (o *Options) Parse(fs *FlagSet, args ...string) error {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		return fmt.Errorf("parsing flags, %w", err)
	}

	if !lo.Contains(validLogLevels, o.LogLevel) {
		return fmt.Errorf("validating cli flags / env vars, invalid log level %q", o.LogLevel)
	}
	gates, err := ParseFeatureGates(o.FeatureGates.inputStr)
	if err != nil {
		return fmt.Errorf("parsing feature gates, %w", err)
	}
	o.FeatureGates = gates
	return nil
}

func (o *Options) ToContext(ctx context.Context) context.Context {
	return ToContext(ctx, o)
}

func ParseFeatureGates(gateStr string) (FeatureGates, error) {
	gateMap := map[string]bool{}
	gates := FeatureGates{}

	// Parses feature gates with the upstream mechanism. This is meant to be used with flag directly but this enables
	// simple merging with environment vars.
	if err := cliflag.NewMapStringBool(&gateMap).Set(gateStr); err != nil {
		return gates, err
	}
	if val, ok := gateMap["Drift"]; ok {
		gates.Drift = val
	}

	return gates, nil
}

func ToContext(ctx context.Context, opts *Options) context.Context {
	return context.WithValue(ctx, optionsKey{}, opts)
}

func FromContext(ctx context.Context) *Options {
	retval := ctx.Value(optionsKey{})
	if retval == nil {
		// This is a developer error if this happens, so we should panic
		panic("options doesn't exist in context")
	}
	return retval.(*Options)
}
