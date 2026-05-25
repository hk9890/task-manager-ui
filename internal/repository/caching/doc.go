// Package caching provides a CachingRepository decorator. See caching.go for
// full package-level documentation.
//
// # Variable ClosedLimit caching strategy (iwvm.3 decision)
//
// When the caller requests a Dashboard with a different ClosedLimit than the
// last successful fetch, the cache treats this as a miss and re-fetches from the
// backing repository (option a — re-fetch on change). The alternative (option b
// — high-water slice) would store the largest-ever limit and slice the cache to
// satisfy smaller requests, but this complicates the interaction with ForceFresh
// (tracked in fbea): a force-fresh request could silently receive a sliced result
// from a previous high-water fetch instead of fresh data. Option a keeps the two
// invalidation axes (dirty flag and ClosedLimit change) orthogonal: dirty=true
// always triggers a backing call regardless of ClosedLimit, and a ClosedLimit
// change always triggers a backing call regardless of the dirty flag.
package caching
