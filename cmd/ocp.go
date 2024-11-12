// Copyright 2022 The Kube-burner Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"embed"
	"fmt"
	"os"
	"time"

	uid "github.com/google/uuid"
	"github.com/kube-burner/kube-burner/pkg/config"
	"github.com/kube-burner/kube-burner/pkg/util"
	"github.com/kube-burner/kube-burner/pkg/workloads"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"kube-burner.io/ocp"
)

//go:embed config/*
var ocpConfig embed.FS

const configDir = "config"

func openShiftCmd() *cobra.Command {
	var workloadConfig workloads.Config
	var wh workloads.WorkloadHelper
	var esServer, esIndex string
	var QPS, burst int
	var gc, gcMetrics, alerting, checkHealth, localIndexing, extract bool
	ocpCmd := &cobra.Command{
		Use:  "kube-burner-ocp",
		Long: `kube-burner plugin designed to be used with OpenShift clusters as a quick way to run well-known workloads`,
	}
	ocpCmd.PersistentFlags().StringVar(&esServer, "es-server", "", "Elastic Search endpoint")
	ocpCmd.PersistentFlags().StringVar(&esIndex, "es-index", "", "Elastic Search index")
	ocpCmd.PersistentFlags().BoolVar(&localIndexing, "local-indexing", false, "Enable local indexing")
	ocpCmd.PersistentFlags().StringVar(&workloadConfig.MetricsEndpoint, "metrics-endpoint", "", "YAML file with a list of metric endpoints, overrides the es-server and es-index flags")
	ocpCmd.PersistentFlags().BoolVar(&alerting, "alerting", true, "Enable alerting")
	ocpCmd.PersistentFlags().BoolVar(&checkHealth, "check-health", true, "Check cluster health before job")
	ocpCmd.PersistentFlags().StringVar(&workloadConfig.UUID, "uuid", uid.NewString(), "Benchmark UUID")
	ocpCmd.PersistentFlags().DurationVar(&workloadConfig.Timeout, "timeout", 4*time.Hour, "Benchmark timeout")
	ocpCmd.PersistentFlags().IntVar(&QPS, "qps", 20, "QPS")
	ocpCmd.PersistentFlags().IntVar(&burst, "burst", 20, "Burst")
	ocpCmd.PersistentFlags().BoolVar(&gc, "gc", true, "Garbage collect created resources")
	ocpCmd.PersistentFlags().BoolVar(&gcMetrics, "gc-metrics", false, "Collect metrics during garbage collection")
	ocpCmd.PersistentFlags().StringVar(&workloadConfig.UserMetadata, "user-metadata", "", "User provided metadata file, in YAML format")
	ocpCmd.PersistentFlags().BoolVar(&extract, "extract", false, "Extract workload in the current directory")
	ocpCmd.PersistentFlags().SortFlags = false
	ocpCmd.MarkFlagsRequiredTogether("es-server", "es-index")
	ocpCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		if cmd.Name() == "version" {
			return
		}
		util.ConfigureLogging(cmd)
		if extract {
			if err := workloads.ExtractWorkload(ocpConfig, configDir, cmd.Name(), "alerts.yml", "metrics.yml", "metrics-aggregated.yml", "metrics-report.yml"); err != nil {
				log.Fatal(err.Error())
			}
			os.Exit(0)
		} else {
			util.SetupLogging("ocp-" + workloadConfig.UUID)
		}
		if checkHealth && (cmd.Name() != "cluster-health" || cmd.Name() == "index") {
			ocp.ClusterHealthCheck()
		}
		workloadConfig.ConfigDir = configDir
		kubeClientProvider := config.NewKubeClientProvider("", "")
		wh = workloads.NewWorkloadHelper(workloadConfig, ocpConfig, kubeClientProvider)
		envVars := map[string]string{
			"UUID":       workloadConfig.UUID,
			"QPS":        fmt.Sprintf("%d", QPS),
			"BURST":      fmt.Sprintf("%d", burst),
			"GC":         fmt.Sprintf("%v", gc),
			"GC_METRICS": fmt.Sprintf("%v", gcMetrics),
		}
		envVars["LOCAL_INDEXING"] = fmt.Sprintf("%v", localIndexing)
		if alerting {
			envVars["ALERTS"] = "alerts.yml"
		} else {
			envVars["ALERTS"] = ""
		}
		// If metricsEndpoint is not set, use values from flags
		if workloadConfig.MetricsEndpoint == "" {
			envVars["ES_SERVER"] = esServer
			envVars["ES_INDEX"] = esIndex
		}
		for k, v := range envVars {
			os.Setenv(k, v)
		}
		if err := ocp.GatherMetadata(&wh, alerting); err != nil {
			log.Fatal(err.Error())
		}
	}
	ocpCmd.AddCommand(
		ocp.NewWorkload(ocp.NewClusterDensity(&wh, "cluster-density-v2"), wh),
		ocp.NewWorkload(ocp.NewClusterDensity(&wh, "cluster-density-ms"), wh),
		ocp.NewWorkload(ocp.NewCrdScale(), wh),
		ocp.NewWorkload(ocp.NewNetworkPolicy("networkpolicy-multitenant"), wh),
		ocp.NewWorkload(ocp.NewNetworkPolicy("networkpolicy-matchlabels"), wh),
		ocp.NewWorkload(ocp.NewNetworkPolicy("networkpolicy-matchexpressions"), wh),
		ocp.NewWorkload(ocp.NewNodeDensity(&wh), wh),
		ocp.NewWorkload(ocp.NewNodeDensityHeavy(&wh), wh),
		ocp.NewWorkload(ocp.NewNodeDensityCNI(&wh), wh),
		ocp.NewWorkload(ocp.NewPVCDensity(), wh),
		ocp.NewWorkload(ocp.NewRDSCore(&wh), wh),
		ocp.NewWorkload(ocp.NewWebBurner("web-burner-init"), wh),
		ocp.NewWorkload(ocp.NewWebBurner("web-burner-node-density"), wh),
		ocp.NewWorkload(ocp.NewWebBurner("web-burner-cluster-density"), wh),
		ocp.NewWorkload(ocp.NewEgressIP("egressip"), wh),
		ocp.NewWorkload(ocp.NewUDNDensityPods(&wh), wh),
		ocp.CustomWorkload(&wh),
		ocp.NewWorkersScale(&wh.MetricsEndpoint, &wh.MetadataAgent),
		ocp.NewIndex(&wh.MetricsEndpoint, &wh.MetadataAgent),
		ocp.ClusterHealth(),
	)
	util.SetupCmd(ocpCmd)
	return ocpCmd
}

func main() {
	if openShiftCmd().Execute() != nil {
		os.Exit(1)
	}
}
