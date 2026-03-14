// Package runtime provides production runtime tuning for containerised
// deployments. Importing this package as a side effect automatically sets
// GOMAXPROCS to match the Linux CPU quota (cgroup v1/v2), which prevents
// over-scheduling in Kubernetes pods with fractional CPU limits.
//
// Usage in main.go:
//
//	import _ "github.com/vincent-tien/wolf-core/infra/runtime"
package runtime

import _ "go.uber.org/automaxprocs"
