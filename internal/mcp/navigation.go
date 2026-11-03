package mcp

import (
	"fmt"
	"slices"
)

// Navigation describes capabilities, never authorization or renderer selection.
type Navigation struct {
	ResourceTypes []string `json:"resource_types"`
	Signals       []string `json:"signals"`
	OperationKind string   `json:"operation_kind,omitempty"`
	ResultKind    []string `json:"result_kind,omitempty"`
}

var telemetryResourceTypes = []string{"host", "kubernetes_cluster", "kubernetes_namespace", "application", "monitoring_target"}

// Reviewed declarations for the production native registry. Synthetic registries
// may supply their own metadata; production composition rejects missing entries.
var nativeNavigation = map[string]Navigation{
	"alert.channel.list":                           {[]string{"alert"}, []string{}, "discover", []string{"resource_list"}},
	"alert.event.acknowledge.preview":              {[]string{"alert"}, []string{}, "preview", []string{"pending_action"}},
	"alert.event.assign.preview":                   {[]string{"alert"}, []string{}, "preview", []string{"pending_action"}},
	"alert.event.comment.preview":                  {[]string{"alert"}, []string{}, "preview", []string{"pending_action"}},
	"alert.event.get":                              {[]string{"alert"}, []string{}, "query", []string{"resource_detail"}},
	"alert.event.query":                            {[]string{"alert"}, []string{}, "query", []string{"resource_list"}},
	"alert.grouping.list":                          {[]string{"alert"}, []string{}, "discover", []string{"resource_list"}},
	"alert.history.query":                          {[]string{"alert"}, []string{}, "query", []string{"resource_list"}},
	"alert.inhibition.create.preview":              {[]string{"alert"}, []string{}, "preview", []string{"pending_action"}},
	"alert.inhibition.delete.preview":              {[]string{"alert"}, []string{}, "preview", []string{"pending_action"}},
	"alert.inhibition.disable.preview":             {[]string{"alert"}, []string{}, "preview", []string{"pending_action"}},
	"alert.inhibition.enable.preview":              {[]string{"alert"}, []string{}, "preview", []string{"pending_action"}},
	"alert.inhibition.list":                        {[]string{"alert"}, []string{}, "discover", []string{"resource_list"}},
	"alert.inhibition.update.preview":              {[]string{"alert"}, []string{}, "preview", []string{"pending_action"}},
	"alert.notification.query":                     {[]string{"alert"}, []string{}, "query", []string{"resource_list"}},
	"alert.notification.resend.preview":            {[]string{"alert"}, []string{}, "preview", []string{"pending_action"}},
	"alert.notification.test.preview":              {[]string{"alert"}, []string{}, "preview", []string{"pending_action"}},
	"alert.policy.create.preview":                  {[]string{"alert"}, []string{}, "preview", []string{"pending_action"}},
	"alert.policy.delete.preview":                  {[]string{"alert"}, []string{}, "preview", []string{"pending_action"}},
	"alert.policy.disable.preview":                 {[]string{"alert"}, []string{}, "preview", []string{"pending_action"}},
	"alert.policy.enable.preview":                  {[]string{"alert"}, []string{}, "preview", []string{"pending_action"}},
	"alert.policy.get":                             {[]string{"alert"}, []string{}, "query", []string{"resource_detail"}},
	"alert.policy.list":                            {[]string{"alert"}, []string{}, "discover", []string{"resource_list"}},
	"alert.policy.update.preview":                  {[]string{"alert"}, []string{}, "preview", []string{"pending_action"}},
	"alert.route.create.preview":                   {[]string{"alert"}, []string{}, "preview", []string{"pending_action"}},
	"alert.route.delete.preview":                   {[]string{"alert"}, []string{}, "preview", []string{"pending_action"}},
	"alert.route.disable.preview":                  {[]string{"alert"}, []string{}, "preview", []string{"pending_action"}},
	"alert.route.enable.preview":                   {[]string{"alert"}, []string{}, "preview", []string{"pending_action"}},
	"alert.route.list":                             {[]string{"alert"}, []string{}, "discover", []string{"resource_list"}},
	"alert.route.update.preview":                   {[]string{"alert"}, []string{}, "preview", []string{"pending_action"}},
	"alert.settings.list":                          {[]string{"alert"}, []string{}, "discover", []string{"resource_list"}},
	"alert.silence.create.preview":                 {[]string{"alert"}, []string{}, "preview", []string{"pending_action"}},
	"alert.silence.delete.preview":                 {[]string{"alert"}, []string{}, "preview", []string{"pending_action"}},
	"alert.silence.disable.preview":                {[]string{"alert"}, []string{}, "preview", []string{"pending_action"}},
	"alert.silence.enable.preview":                 {[]string{"alert"}, []string{}, "preview", []string{"pending_action"}},
	"alert.silence.list":                           {[]string{"alert"}, []string{}, "discover", []string{"resource_list"}},
	"alert.silence.update.preview":                 {[]string{"alert"}, []string{}, "preview", []string{"pending_action"}},
	"alert.template.list":                          {[]string{"alert"}, []string{}, "discover", []string{"resource_list"}},
	"application.change.preview":                   {[]string{"application"}, []string{}, "preview", []string{"pending_action"}},
	"application.endpoint.connection":              {[]string{"application"}, []string{}, "query", []string{"resource_detail"}},
	"application.endpoint.list":                    {[]string{"application"}, []string{}, "discover", []string{"resource_list"}},
	"application.get":                              {[]string{"application"}, []string{}, "query", []string{"resource_detail"}},
	"application.key.list":                         {[]string{"application"}, []string{}, "discover", []string{"resource_list"}},
	"application.list":                             {[]string{"application"}, []string{}, "discover", []string{"resource_list"}},
	"application.service.list":                     {[]string{"application"}, []string{}, "discover", []string{"resource_list"}},
	"connector.bastion.create.preview":             {[]string{"connector"}, []string{}, "preview", []string{"pending_action"}},
	"connector.bastion.delete.preview":             {[]string{"connector"}, []string{}, "preview", []string{"pending_action"}},
	"connector.bastion.enrollment_rotate.preview":  {[]string{"connector"}, []string{}, "preview", []string{"pending_action"}},
	"connector.bastion.replacement.preview":        {[]string{"connector"}, []string{}, "preview", []string{"pending_action"}},
	"connector.bastion.update.preview":             {[]string{"connector"}, []string{}, "preview", []string{"pending_action"}},
	"connector.get":                                {[]string{"connector"}, []string{}, "query", []string{"resource_detail"}},
	"connector.install.retry.preview":              {[]string{"connector"}, []string{}, "preview", []string{"pending_action"}},
	"connector.list":                               {[]string{"connector"}, []string{}, "discover", []string{"resource_list"}},
	"connector.uninstall.preview":                  {[]string{"connector"}, []string{}, "preview", []string{"pending_action"}},
	"host.collector.configure.preview":             {[]string{"host"}, []string{}, "preview", []string{"pending_action"}},
	"host.collector.install.preview":               {[]string{"host"}, []string{}, "preview", []string{"pending_action"}},
	"host.collector.repair.preview":                {[]string{"host"}, []string{}, "preview", []string{"pending_action"}},
	"host.collector.uninstall.preview":             {[]string{"host"}, []string{}, "preview", []string{"pending_action"}},
	"host.collector.upgrade.preview":               {[]string{"host"}, []string{}, "preview", []string{"pending_action"}},
	"host.create.preview":                          {[]string{"host"}, []string{}, "preview", []string{"pending_action"}},
	"host.delete.preview":                          {[]string{"host"}, []string{}, "preview", []string{"pending_action"}},
	"host.get":                                     {[]string{"host"}, []string{}, "query", []string{"resource_detail"}},
	"host.install.retry.preview":                   {[]string{"host"}, []string{}, "preview", []string{"pending_action"}},
	"host.list":                                    {[]string{"host"}, []string{}, "discover", []string{"resource_list"}},
	"host.removal.preview":                         {[]string{"host"}, []string{}, "preview", []string{"pending_action"}},
	"host.update.preview":                          {[]string{"host"}, []string{}, "preview", []string{"pending_action"}},
	"kubernetes.cluster.create.preview":            {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{}, "preview", []string{"pending_action"}},
	"kubernetes.cluster.delete.preview":            {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{}, "preview", []string{"pending_action"}},
	"kubernetes.cluster.get":                       {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{}, "query", []string{"resource_detail"}},
	"kubernetes.cluster.list":                      {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{}, "discover", []string{"resource_list"}},
	"kubernetes.cluster.update.preview":            {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{}, "preview", []string{"pending_action"}},
	"kubernetes.collector.configure.preview":       {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{}, "preview", []string{"pending_action"}},
	"kubernetes.collector.install.preview":         {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{}, "preview", []string{"pending_action"}},
	"kubernetes.collector.repair.preview":          {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{}, "preview", []string{"pending_action"}},
	"kubernetes.collector.uninstall.preview":       {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{}, "preview", []string{"pending_action"}},
	"kubernetes.collector.upgrade.preview":         {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{}, "preview", []string{"pending_action"}},
	"kubernetes.daemonset.list":                    {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{}, "discover", []string{"resource_list"}},
	"kubernetes.deployment.list":                   {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{}, "discover", []string{"resource_list"}},
	"kubernetes.drain.cancel.preview":              {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{}, "preview", []string{"pending_action"}},
	"kubernetes.helm.change.preview":               {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{}, "preview", []string{"pending_action"}},
	"kubernetes.helm.history":                      {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{}, "query", []string{"resource_list"}},
	"kubernetes.helm.list":                         {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{}, "discover", []string{"resource_list"}},
	"kubernetes.namespace.list":                    {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{}, "discover", []string{"resource_list"}},
	"kubernetes.node.list":                         {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{}, "discover", []string{"resource_list"}},
	"kubernetes.node_host_binding.confirm.preview": {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{}, "preview", []string{"pending_action"}},
	"kubernetes.object.change.preview":             {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{}, "preview", []string{"pending_action"}},
	"kubernetes.object.get":                        {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{}, "query", []string{"resource_detail"}},
	"kubernetes.object.list":                       {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{}, "discover", []string{"resource_list"}},
	"kubernetes.operation.get":                     {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{}, "query", []string{"resource_detail"}},
	"kubernetes.operation.list":                    {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{}, "discover", []string{"resource_list"}},
	"kubernetes.pod.list":                          {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{}, "discover", []string{"resource_list"}},
	"kubernetes.pod.logs":                          {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{"logs"}, "query", []string{"logs"}},
	"kubernetes.service.list":                      {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{}, "discover", []string{"resource_list"}},
	"kubernetes.statefulset.list":                  {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{}, "discover", []string{"resource_list"}},
	"kubernetes.workbench.types":                   {[]string{"kubernetes_cluster", "kubernetes_namespace"}, []string{}, "discover", []string{"resource_detail"}},
	"monitoring.builtin.change.preview":            {[]string{"monitoring_target", "host", "kubernetes_cluster", "application"}, []string{}, "preview", []string{"pending_action"}},
	"monitoring.builtin.list":                      {[]string{"monitoring_target", "host", "kubernetes_cluster", "application"}, []string{}, "discover", []string{"resource_list"}},
	"monitoring.collection.change.preview":         {[]string{"monitoring_target", "host", "kubernetes_cluster", "application"}, []string{}, "preview", []string{"pending_action"}},
	"monitoring.collection.form":                   {[]string{"monitoring_target", "host", "kubernetes_cluster", "application"}, []string{}, "prepare", []string{"interaction"}},
	"monitoring.collection.get":                    {[]string{"monitoring_target", "host", "kubernetes_cluster", "application"}, []string{}, "query", []string{"resource_detail"}},
	"monitoring.collection.list":                   {[]string{"monitoring_target", "host", "kubernetes_cluster", "application"}, []string{}, "discover", []string{"resource_list"}},
	"monitoring.collector.choices":                 {[]string{"monitoring_target", "host", "kubernetes_cluster", "application"}, []string{}, "discover", []string{"resource_detail"}},
	"monitoring.integration.list":                  {[]string{"monitoring_target", "host", "kubernetes_cluster", "application"}, []string{}, "discover", []string{"resource_list"}},
	"monitoring.target.list":                       {[]string{"monitoring_target", "host", "kubernetes_cluster", "application"}, []string{}, "discover", []string{"resource_list"}},
	"pending_action.cancel":                        {[]string{}, []string{}, "control", []string{"operation_status"}},
	"pending_action.get":                           {[]string{}, []string{}, "query", []string{"operation_status"}},
	"telemetry.collector.get":                      {[]string{"host", "kubernetes_cluster"}, []string{}, "query", []string{"resource_detail"}},
	"telemetry.collector.list":                     {[]string{"host", "kubernetes_cluster"}, []string{}, "discover", []string{"resource_list"}},
	"telemetry.dashboard.catalog":                  {[]string{"dashboard"}, []string{"metrics", "logs", "traces"}, "discover", []string{"metric_catalog", "fields", "values"}},
	"telemetry.dashboard.catalog.resources":        {[]string{"dashboard"}, []string{}, "discover", []string{"resource_detail"}},
	"telemetry.dashboard.context.candidates":       {[]string{"dashboard"}, []string{}, "query", []string{"resource_detail"}},
	"telemetry.dashboard.context.resolve":          {[]string{"dashboard"}, []string{}, "query", []string{"resource_detail"}},
	"telemetry.dashboard.draft.create":             {[]string{"dashboard"}, []string{}, "prepare", []string{"dashboard_draft"}},
	"telemetry.dashboard.draft.get":                {[]string{"dashboard"}, []string{}, "query", []string{"resource_detail"}},
	"telemetry.dashboard.draft.save":               {[]string{"dashboard"}, []string{}, "prepare", []string{"dashboard_draft"}},
	"telemetry.dashboard.draft.validate":           {[]string{"dashboard"}, []string{}, "prepare", []string{"dashboard_draft"}},
	"telemetry.dashboard.get":                      {[]string{"dashboard"}, []string{}, "query", []string{"resource_detail"}},
	"telemetry.dashboard.list":                     {[]string{"dashboard"}, []string{}, "discover", []string{"resource_list"}},
	"telemetry.dashboard.publish.preview":          {[]string{"dashboard"}, []string{}, "preview", []string{"pending_action"}},
	"telemetry.dashboard.query":                    {[]string{"dashboard"}, []string{"metrics", "logs", "traces"}, "query", []string{"dashboard_result"}},
	"telemetry.dashboard.query.cancel":             {[]string{"dashboard"}, []string{}, "control", []string{"operation_status"}},
	"telemetry.dashboard.query.get":                {[]string{"dashboard"}, []string{"metrics", "logs", "traces"}, "query", []string{"dashboard_result"}},
	"telemetry.dashboard.query.resume":             {[]string{"dashboard"}, []string{}, "control", []string{"operation_status"}},
	"telemetry.kql.query":                          {[]string{"host", "kubernetes_cluster", "kubernetes_namespace", "application", "monitoring_target"}, []string{"logs"}, "query", []string{"logs", "table"}},
	"telemetry.overview":                           {[]string{"host", "kubernetes_cluster", "kubernetes_namespace", "application", "monitoring_target"}, []string{"metrics", "logs", "traces"}, "query", []string{"coverage"}},
	"telemetry.promql.query":                       {[]string{"host", "kubernetes_cluster", "kubernetes_namespace", "application", "monitoring_target"}, []string{"metrics"}, "query", []string{"timeseries", "instant_vector", "scalar"}},
	"telemetry.skywalking.trace":                   {[]string{"host", "kubernetes_cluster", "kubernetes_namespace", "application", "monitoring_target"}, []string{"traces"}, "query", []string{"traces", "apm"}},
	"workflow.import_result":                       {[]string{}, []string{}, "deliver", []string{"file"}},
	"workflow.plan.propose":                        {[]string{}, []string{}, "prepare", []string{"interaction"}},
	"workflow.publish_file":                        {[]string{}, []string{}, "deliver", []string{"file"}},
	"observe.resources.list":                       {telemetryResourceTypes, []string{"metrics", "logs", "traces"}, "discover", []string{"resource_list"}},
	"observe.capabilities.get":                     {telemetryResourceTypes, []string{"metrics", "logs", "traces"}, "discover", []string{"resource_capabilities"}},
	"observe.catalog.query":                        {telemetryResourceTypes, []string{"metrics", "logs", "traces"}, "discover", []string{"metric_catalog"}},
}

func (n Navigation) clone() Navigation {
	n.ResourceTypes = slices.Clone(n.ResourceTypes)
	n.Signals = slices.Clone(n.Signals)
	n.ResultKind = slices.Clone(n.ResultKind)
	return n
}

func (n Navigation) Validate() error {
	if !slices.Contains([]string{"discover", "query", "prepare", "preview", "control", "deliver"}, n.OperationKind) || len(n.ResultKind) == 0 {
		return fmt.Errorf("missing or invalid tool navigation")
	}
	for _, signal := range n.Signals {
		if !slices.Contains([]string{"metrics", "logs", "traces"}, signal) {
			return fmt.Errorf("invalid navigation signal %s", signal)
		}
	}
	for _, kind := range n.ResourceTypes {
		if !slices.Contains([]string{"host", "kubernetes_cluster", "kubernetes_namespace", "application", "monitoring_target", "dashboard", "connector", "alert"}, kind) {
			return fmt.Errorf("invalid navigation resource %s", kind)
		}
	}
	for _, kind := range n.ResultKind {
		if !slices.Contains([]string{"resource_detail", "resource_list", "pending_action", "operation_status", "file", "interaction", "dashboard_draft", "timeseries", "instant_vector", "scalar", "logs", "table", "traces", "apm", "coverage", "dashboard_result", "metric_catalog", "fields", "values", "resource_capabilities"}, kind) {
			return fmt.Errorf("invalid navigation result %s", kind)
		}
	}
	return nil
}
