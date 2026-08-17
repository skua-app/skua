package glance

// Retention exposes the unexported retention window to this package's
// external tests so they can derive timestamps that sit clearly inside
// or clearly outside the cutoff without restating the window a second
// time. Files ending in _test.go are only compiled into the test
// binary, so this adds nothing to the production build.
const Retention = glanceRetention
