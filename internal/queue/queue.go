// Package queue holds the shared Redis/Asynq bootstrap for trackid's
// background job pipeline (embedding computation, matching, clustering --
// registered incrementally as each lands in cmd/worker's mux).
package queue

import "github.com/hibiken/asynq"

// OpenClient parses redisURL and returns an Asynq client for enqueuing tasks.
// Callers must Close it when done.
func OpenClient(redisURL string) (*asynq.Client, error) {
	opt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		return nil, err
	}
	return asynq.NewClient(opt), nil
}

// OpenServer parses redisURL and returns an Asynq server ready to Run with a
// registered ServeMux.
func OpenServer(redisURL string, cfg asynq.Config) (*asynq.Server, error) {
	opt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		return nil, err
	}
	return asynq.NewServer(opt, cfg), nil
}
